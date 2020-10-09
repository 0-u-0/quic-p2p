// +build !js

// Package webrtc implements the WebRTC 1.0 as defined in W3C WebRTC specification document.
package webrtc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	mathRand "math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/rtcp"
	"github.com/pion/sdp/v2"

	"github.com/pion/webrtc/v2/internal/util"
	"github.com/pion/webrtc/v2/pkg/rtcerr"
)

// PeerConnection represents a WebRTC connection that establishes a
// peer-to-peer communications with another PeerConnection instance in a
// browser, or to another endpoint implementing the required protocols.
type PeerConnection struct {
	statsID string
	mu      sync.RWMutex

	configuration Configuration

	currentLocalDescription  *SessionDescription
	pendingLocalDescription  *SessionDescription
	currentRemoteDescription *SessionDescription
	pendingRemoteDescription *SessionDescription
	signalingState           SignalingState
	iceConnectionState       ICEConnectionState
	connectionState          PeerConnectionState

	idpLoginURL *string

	isClosed          *atomicBool
	negotiationNeeded bool

	lastOffer  string
	lastAnswer string

	rtpTransceivers []*RTPTransceiver

	onSignalingStateChangeHandler     func(SignalingState)
	onICEConnectionStateChangeHandler func(ICEConnectionState)
	onConnectionStateChangeHandler    func(PeerConnectionState)
	onTrackHandler                    func(*Track, *RTPReceiver)
	onDataChannelHandler              func(*DataChannel)

	iceGatherer   *ICEGatherer
	iceTransport  *ICETransport
	dtlsTransport *DTLSTransport

	// A reference to the associated API state used by this connection
	api *API
	log logging.LeveledLogger
}


// initConfiguration defines validation of the specified Configuration and
// its assignment to the internal configuration variable. This function differs
// from its SetConfiguration counterpart because most of the checks do not
// include verification statements related to the existing state. Thus the
// function describes only minor verification of some the struct variables.
func (pc *PeerConnection) initConfiguration(configuration Configuration) error {
	if configuration.PeerIdentity != "" {
		pc.configuration.PeerIdentity = configuration.PeerIdentity
	}

	// https://www.w3.org/TR/webrtc/#constructor (step #3)
	if len(configuration.Certificates) > 0 {
		now := time.Now()
		for _, x509Cert := range configuration.Certificates {
			if !x509Cert.Expires().IsZero() && now.After(x509Cert.Expires()) {
				return &rtcerr.InvalidAccessError{Err: ErrCertificateExpired}
			}
			pc.configuration.Certificates = append(pc.configuration.Certificates, x509Cert)
		}
	} else {
		sk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return &rtcerr.UnknownError{Err: err}
		}
		certificate, err := GenerateCertificate(sk)
		if err != nil {
			return err
		}
		pc.configuration.Certificates = []Certificate{*certificate}
	}

	if configuration.BundlePolicy != BundlePolicy(Unknown) {
		pc.configuration.BundlePolicy = configuration.BundlePolicy
	}

	if configuration.RTCPMuxPolicy != RTCPMuxPolicy(Unknown) {
		pc.configuration.RTCPMuxPolicy = configuration.RTCPMuxPolicy
	}

	if configuration.ICECandidatePoolSize != 0 {
		pc.configuration.ICECandidatePoolSize = configuration.ICECandidatePoolSize
	}

	if configuration.ICETransportPolicy != ICETransportPolicy(Unknown) {
		pc.configuration.ICETransportPolicy = configuration.ICETransportPolicy
	}

	if configuration.SDPSemantics != SDPSemantics(Unknown) {
		pc.configuration.SDPSemantics = configuration.SDPSemantics
	}

	sanitizedICEServers := configuration.getICEServers()
	if len(sanitizedICEServers) > 0 {
		for _, server := range sanitizedICEServers {
			if err := server.validate(); err != nil {
				return err
			}
		}
		pc.configuration.ICEServers = sanitizedICEServers
	}

	return nil
}

// OnSignalingStateChange sets an event handler which is invoked when the
// peer connection's signaling state changes
func (pc *PeerConnection) OnSignalingStateChange(f func(SignalingState)) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.onSignalingStateChangeHandler = f
}

func (pc *PeerConnection) onSignalingStateChange(newState SignalingState) {
	pc.mu.RLock()
	hdlr := pc.onSignalingStateChangeHandler
	pc.mu.RUnlock()

	pc.log.Infof("signaling state changed to %s", newState)
	if hdlr != nil {
		go hdlr(newState)
	}
}

// OnDataChannel sets an event handler which is invoked when a data
// channel message arrives from a remote peer.
func (pc *PeerConnection) OnDataChannel(f func(*DataChannel)) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.onDataChannelHandler = f
}

// OnICECandidate sets an event handler which is invoked when a new ICE
// candidate is found.
func (pc *PeerConnection) OnICECandidate(f func(*ICECandidate)) {
	pc.iceGatherer.OnLocalCandidate(f)
}

// OnICEGatheringStateChange sets an event handler which is invoked when the
// ICE candidate gathering state has changed.
func (pc *PeerConnection) OnICEGatheringStateChange(f func(ICEGathererState)) {
	pc.iceGatherer.OnStateChange(f)
}

// OnTrack sets an event handler which is called when remote track
// arrives from a remote peer.
func (pc *PeerConnection) OnTrack(f func(*Track, *RTPReceiver)) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.onTrackHandler = f
}

func (pc *PeerConnection) onTrack(t *Track, r *RTPReceiver) {
	pc.mu.RLock()
	hdlr := pc.onTrackHandler
	pc.mu.RUnlock()

	pc.log.Debugf("got new track: %+v", t)
	if hdlr != nil && t != nil {
		go hdlr(t, r)
	}
}

// OnICEConnectionStateChange sets an event handler which is called
// when an ICE connection state is changed.
func (pc *PeerConnection) OnICEConnectionStateChange(f func(ICEConnectionState)) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.onICEConnectionStateChangeHandler = f
}

func (pc *PeerConnection) onICEConnectionStateChange(cs ICEConnectionState) {
	pc.mu.Lock()
	pc.iceConnectionState = cs
	hdlr := pc.onICEConnectionStateChangeHandler
	pc.mu.Unlock()

	pc.log.Infof("ICE connection state changed: %s", cs)
	if hdlr != nil {
		go hdlr(cs)
	}
}

// OnConnectionStateChange sets an event handler which is called
// when the PeerConnectionState has changed
func (pc *PeerConnection) OnConnectionStateChange(f func(PeerConnectionState)) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.onConnectionStateChangeHandler = f
}

// GetConfiguration returns a Configuration object representing the current
// configuration of this PeerConnection object. The returned object is a
// copy and direct mutation on it will not take affect until SetConfiguration
// has been called with Configuration passed as its only argument.
// https://www.w3.org/TR/webrtc/#dom-rtcpeerconnection-getconfiguration
func (pc *PeerConnection) GetConfiguration() Configuration {
	return pc.configuration
}

func (pc *PeerConnection) getStatsID() string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.statsID
}

func (pc *PeerConnection) createICEGatherer() (*ICEGatherer, error) {


	return nil, nil
}

// Update the PeerConnectionState given the state of relevant transports
// https://www.w3.org/TR/webrtc/#rtcpeerconnectionstate-enum
func (pc *PeerConnection) updateConnectionState(iceConnectionState ICEConnectionState, dtlsTransportState DTLSTransportState) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	connectionState := PeerConnectionStateNew
	switch {
	// The RTCPeerConnection object's [[IsClosed]] slot is true.
	case pc.isClosed.get():
		connectionState = PeerConnectionStateClosed

	// Any of the RTCIceTransports or RTCDtlsTransports are in a "failed" state.
	case iceConnectionState == ICEConnectionStateFailed || dtlsTransportState == DTLSTransportStateFailed:
		connectionState = PeerConnectionStateFailed

	// Any of the RTCIceTransports or RTCDtlsTransports are in the "disconnected"
	// state and none of them are in the "failed" or "connecting" or "checking" state.  */
	case iceConnectionState == ICEConnectionStateDisconnected:
		connectionState = PeerConnectionStateDisconnected

	// All RTCIceTransports and RTCDtlsTransports are in the "connected", "completed" or "closed"
	// state and at least one of them is in the "connected" or "completed" state.
	case iceConnectionState == ICEConnectionStateConnected && dtlsTransportState == DTLSTransportStateConnected:
		connectionState = PeerConnectionStateConnected

	//  Any of the RTCIceTransports or RTCDtlsTransports are in the "connecting" or
	// "checking" state and none of them is in the "failed" state.
	case iceConnectionState == ICEConnectionStateChecking && dtlsTransportState == DTLSTransportStateConnecting:
		connectionState = PeerConnectionStateConnecting
	}

	if pc.connectionState == connectionState {
		return
	}

	pc.log.Infof("peer connection state changed: %s", connectionState)
	pc.connectionState = connectionState
	hdlr := pc.onConnectionStateChangeHandler
	if hdlr != nil {
		go hdlr(connectionState)
	}
}

func (pc *PeerConnection) createICETransport() *ICETransport {
	t := pc.api.NewICETransport(pc.iceGatherer)
	t.OnConnectionStateChange(func(state ICETransportState) {
		var cs ICEConnectionState
		switch state {
		case ICETransportStateNew:
			cs = ICEConnectionStateNew
		case ICETransportStateChecking:
			cs = ICEConnectionStateChecking
		case ICETransportStateConnected:
			cs = ICEConnectionStateConnected
		case ICETransportStateCompleted:
			cs = ICEConnectionStateCompleted
		case ICETransportStateFailed:
			cs = ICEConnectionStateFailed
		case ICETransportStateDisconnected:
			cs = ICEConnectionStateDisconnected
		case ICETransportStateClosed:
			cs = ICEConnectionStateClosed
		default:
			pc.log.Warnf("OnConnectionStateChange: unhandled ICE state: %s", state)
			return
		}
		pc.onICEConnectionStateChange(cs)
		pc.updateConnectionState(cs, pc.dtlsTransport.State())
	})

	return t
}

func (pc *PeerConnection) getPeerDirection(media *sdp.MediaDescription) RTPTransceiverDirection {
	for _, a := range media.Attributes {
		if direction := NewRTPTransceiverDirection(a.Key); direction != RTPTransceiverDirection(Unknown) {
			return direction
		}
	}
	return RTPTransceiverDirection(Unknown)
}

func (pc *PeerConnection) getMidValue(media *sdp.MediaDescription) string {
	for _, attr := range media.Attributes {
		if attr.Key == "mid" {
			return attr.Value
		}
	}
	return ""
}

// Given a direction+type pluck a transceiver from the passed list
// if no entry satisfies the requested type+direction return a inactive Transceiver
func satisfyTypeAndDirection(remoteKind RTPCodecType, remoteDirection RTPTransceiverDirection, localTransceivers []*RTPTransceiver) (*RTPTransceiver, []*RTPTransceiver) {
	// Get direction order from most preferred to least
	getPreferredDirections := func() []RTPTransceiverDirection {
		switch remoteDirection {
		case RTPTransceiverDirectionSendrecv:
			return []RTPTransceiverDirection{RTPTransceiverDirectionRecvonly, RTPTransceiverDirectionSendrecv}
		case RTPTransceiverDirectionSendonly:
			return []RTPTransceiverDirection{RTPTransceiverDirectionRecvonly, RTPTransceiverDirectionSendrecv}
		case RTPTransceiverDirectionRecvonly:
			return []RTPTransceiverDirection{RTPTransceiverDirectionSendonly, RTPTransceiverDirectionSendrecv}
		}
		return []RTPTransceiverDirection{}
	}

	for _, possibleDirection := range getPreferredDirections() {
		for i := range localTransceivers {
			t := localTransceivers[i]
			if t.kind != remoteKind || possibleDirection != t.Direction {
				continue
			}

			return t, append(localTransceivers[:i], localTransceivers[i+1:]...)
		}
	}

	return &RTPTransceiver{
		kind:      remoteKind,
		Direction: RTPTransceiverDirectionInactive,
	}, localTransceivers
}


func (pc *PeerConnection) descriptionIsPlanB(desc *SessionDescription) bool {
	if desc == nil || desc.parsed == nil {
		return false
	}

	detectionRegex := regexp.MustCompile(`(?i)^(audio|video|data)$`)
	for _, media := range desc.parsed.MediaDescriptions {
		if len(detectionRegex.FindStringSubmatch(pc.getMidValue(media))) == 2 {
			return true
		}
	}
	return false
}

type incomingTrack struct {
	kind  RTPCodecType
	label string
	id    string
	ssrc  uint32
}

func (pc *PeerConnection) startReceiver(incoming incomingTrack, receiver *RTPReceiver) {
	err := receiver.Receive(RTPReceiveParameters{
		Encodings: RTPDecodingParameters{
			RTPCodingParameters{SSRC: incoming.ssrc},
		}})
	if err != nil {
		pc.log.Warnf("RTPReceiver Receive failed %s", err)
		return
	}

	if err = receiver.Track().determinePayloadType(); err != nil {
		pc.log.Warnf("Could not determine PayloadType for SSRC %d", receiver.Track().SSRC())
		return
	}

	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.currentLocalDescription == nil {
		pc.log.Warnf("SetLocalDescription not called, unable to handle incoming media streams")
		return
	}

	sdpCodec, err := pc.currentLocalDescription.parsed.GetCodecForPayloadType(receiver.Track().PayloadType())
	if err != nil {
		pc.log.Warnf("no codec could be found in RemoteDescription for payloadType %d", receiver.Track().PayloadType())
		return
	}

	codec, err := pc.api.mediaEngine.getCodecSDP(sdpCodec)
	if err != nil {
		pc.log.Warnf("codec %s in not registered", sdpCodec)
		return
	}

	receiver.Track().mu.Lock()
	receiver.Track().id = incoming.id
	receiver.Track().label = incoming.label
	receiver.Track().kind = codec.Type
	receiver.Track().codec = codec
	receiver.Track().mu.Unlock()

	if pc.onTrackHandler != nil {
		pc.onTrack(receiver.Track(), receiver)
	} else {
		pc.log.Warnf("OnTrack unset, unable to handle incoming media streams")
	}
}

// openSRTP opens knows inbound SRTP streams from the RemoteDescription
func (pc *PeerConnection) openSRTP() {
	incomingTracks := map[uint32]incomingTrack{}

	remoteIsPlanB := false
	switch pc.configuration.SDPSemantics {
	case SDPSemanticsPlanB:
		remoteIsPlanB = true
	case SDPSemanticsUnifiedPlanWithFallback:
		remoteIsPlanB = pc.descriptionIsPlanB(pc.RemoteDescription())
	}

	for _, media := range pc.RemoteDescription().parsed.MediaDescriptions {
		for _, attr := range media.Attributes {
			codecType := NewRTPCodecType(media.MediaName.Media)
			if codecType == 0 {
				continue
			}

			if attr.Key == sdp.AttrKeySSRC {
				split := strings.Split(attr.Value, " ")
				ssrc, err := strconv.ParseUint(split[0], 10, 32)
				if err != nil {
					pc.log.Warnf("Failed to parse SSRC: %v", err)
					continue
				}

				trackID := ""
				trackLabel := ""
				if len(split) == 3 && strings.HasPrefix(split[1], "msid:") {
					trackLabel = split[1][len("msid:"):]
					trackID = split[2]
				}

				incomingTracks[uint32(ssrc)] = incomingTrack{codecType, trackLabel, trackID, uint32(ssrc)}
				if trackID != "" && trackLabel != "" {
					break // Remote provided Label+ID, we have all the information we need
				}
			}
		}
	}

	localTransceivers := append([]*RTPTransceiver{}, pc.GetTransceivers()...)
	for ssrc, incoming := range incomingTracks {
		for i := range localTransceivers {
			t := localTransceivers[i]
			switch {
			case incomingTracks[ssrc].kind != t.kind:
				continue
			case t.Direction != RTPTransceiverDirectionRecvonly && t.Direction != RTPTransceiverDirectionSendrecv:
				continue
			case t.Receiver == nil:
				continue
			}

			delete(incomingTracks, ssrc)
			localTransceivers = append(localTransceivers[:i], localTransceivers[i+1:]...)
			go pc.startReceiver(incoming, t.Receiver)
			break
		}
	}

	if remoteIsPlanB {
		for ssrc, incoming := range incomingTracks {
			t, err := pc.AddTransceiver(incoming.kind, RtpTransceiverInit{
				Direction: RTPTransceiverDirectionSendrecv,
			})
			if err != nil {
				pc.log.Warnf("Could not add transceiver for remote SSRC %d: %s", ssrc, err)
				continue
			}
			go pc.startReceiver(incoming, t.Receiver)
		}
	}
}

func (pc *PeerConnection) handleUndeclaredSSRC(ssrc uint32) bool {
	if remoteDescription := pc.RemoteDescription(); remoteDescription != nil {
		if len(remoteDescription.parsed.MediaDescriptions) == 1 {
			onlyMediaSection := remoteDescription.parsed.MediaDescriptions[0]
			for _, a := range onlyMediaSection.Attributes {
				if a.Key == ssrcStr {
					return false
				}
			}

			incoming := incomingTrack{
				ssrc: ssrc,
				kind: RTPCodecTypeVideo,
			}
			if onlyMediaSection.MediaName.Media == RTPCodecTypeAudio.String() {
				incoming.kind = RTPCodecTypeAudio
			}

			t, err := pc.AddTransceiver(incoming.kind, RtpTransceiverInit{
				Direction: RTPTransceiverDirectionSendrecv,
			})
			if err != nil {
				pc.log.Warnf("Could not add transceiver for remote SSRC %d: %s", ssrc, err)
				return false
			}
			go pc.startReceiver(incoming, t.Receiver)
			return true
		}
	}

	return false
}

// drainSRTP pulls and discards RTP/RTCP packets that don't match any a:ssrc lines
// If the remote SDP was only one media section the ssrc doesn't have to be explicitly declared
func (pc *PeerConnection) drainSRTP() {
	go func() {
		for {
			srtpSession, err := pc.dtlsTransport.getSRTPSession()
			if err != nil {
				pc.log.Warnf("drainSRTP failed to open SrtpSession: %v", err)
				return
			}

			_, ssrc, err := srtpSession.AcceptStream()
			if err != nil {
				pc.log.Warnf("Failed to accept RTP %v \n", err)
				return
			}

			if !pc.handleUndeclaredSSRC(ssrc) {
				pc.log.Errorf("Incoming unhandled RTP ssrc(%d)", ssrc)
			}
		}
	}()

	for {
		srtcpSession, err := pc.dtlsTransport.getSRTCPSession()
		if err != nil {
			pc.log.Warnf("drainSRTP failed to open SrtcpSession: %v", err)
			return
		}

		_, ssrc, err := srtcpSession.AcceptStream()
		if err != nil {
			pc.log.Warnf("Failed to accept RTCP %v \n", err)
			return
		}
		pc.log.Errorf("Incoming unhandled RTCP ssrc(%d)", ssrc)
	}
}

// RemoteDescription returns pendingRemoteDescription if it is not null and
// otherwise it returns currentRemoteDescription. This property is used to
// determine if setRemoteDescription has already been called.
// https://www.w3.org/TR/webrtc/#dom-rtcpeerconnection-remotedescription
func (pc *PeerConnection) RemoteDescription() *SessionDescription {
	if pc.pendingRemoteDescription != nil {
		return pc.pendingRemoteDescription
	}
	return pc.currentRemoteDescription
}

// AddICECandidate accepts an ICE candidate string and adds it
// to the existing set of candidates
func (pc *PeerConnection) AddICECandidate(candidate ICECandidateInit) error {
	if pc.RemoteDescription() == nil {
		return &rtcerr.InvalidStateError{Err: ErrNoRemoteDescription}
	}

	candidateValue := strings.TrimPrefix(candidate.Candidate, "candidate:")
	attribute := sdp.NewAttribute("candidate", candidateValue)
	sdpCandidate, err := attribute.ToICECandidate()
	if err != nil {
		return err
	}

	iceCandidate, err := newICECandidateFromSDP(sdpCandidate)
	if err != nil {
		return err
	}

	return pc.iceTransport.AddRemoteCandidate(iceCandidate)
}

// ICEConnectionState returns the ICE connection state of the
// PeerConnection instance.
func (pc *PeerConnection) ICEConnectionState() ICEConnectionState {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	return pc.iceConnectionState
}

// GetSenders returns the RTPSender that are currently attached to this PeerConnection
func (pc *PeerConnection) GetSenders() []*RTPSender {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	result := []*RTPSender{}
	for _, tranceiver := range pc.rtpTransceivers {
		if tranceiver.Sender != nil {
			result = append(result, tranceiver.Sender)
		}
	}
	return result
}

// GetReceivers returns the RTPReceivers that are currently attached to this RTCPeerConnection
func (pc *PeerConnection) GetReceivers() []*RTPReceiver {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	result := []*RTPReceiver{}
	for _, tranceiver := range pc.rtpTransceivers {
		if tranceiver.Receiver != nil {
			result = append(result, tranceiver.Receiver)
		}
	}
	return result
}

// GetTransceivers returns the RTCRtpTransceiver that are currently attached to this RTCPeerConnection
func (pc *PeerConnection) GetTransceivers() []*RTPTransceiver {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	return pc.rtpTransceivers
}

// AddTrack adds a Track to the PeerConnection
func (pc *PeerConnection) AddTrack(track *Track) (*RTPSender, error) {
	if pc.isClosed.get() {
		return nil, &rtcerr.InvalidStateError{Err: ErrConnectionClosed}
	}
	var transceiver *RTPTransceiver
	for _, t := range pc.GetTransceivers() {
		if !t.stopped &&
			t.Sender != nil &&
			!t.Sender.hasSent() &&
			t.Receiver != nil &&
			t.Receiver.Track() != nil &&
			t.Receiver.Track().Kind() == track.Kind() {
			transceiver = t
			break
		}
	}
	if transceiver != nil {
		if err := transceiver.setSendingTrack(track); err != nil {
			return nil, err
		}
	} else {
		receiver, err := pc.api.NewRTPReceiver(track.Kind(), pc.dtlsTransport)
		if err != nil {
			return nil, err
		}

		sender, err := pc.api.NewRTPSender(track, pc.dtlsTransport)
		if err != nil {
			return nil, err
		}
		transceiver = pc.newRTPTransceiver(
			receiver,
			sender,
			RTPTransceiverDirectionSendrecv,
			track.Kind(),
		)
	}

	return transceiver.Sender, nil
}

// AddTransceiver Create a new RTCRtpTransceiver and add it to the set of transceivers.
// Deprecated: Use AddTrack, AddTransceiverFromKind or AddTransceiverFromTrack
func (pc *PeerConnection) AddTransceiver(trackOrKind RTPCodecType, init ...RtpTransceiverInit) (*RTPTransceiver, error) {
	return pc.AddTransceiverFromKind(trackOrKind, init...)
}

// AddTransceiverFromKind Create a new RTCRtpTransceiver(SendRecv or RecvOnly) and add it to the set of transceivers.
func (pc *PeerConnection) AddTransceiverFromKind(kind RTPCodecType, init ...RtpTransceiverInit) (*RTPTransceiver, error) {
	direction := RTPTransceiverDirectionSendrecv
	if len(init) > 1 {
		return nil, fmt.Errorf("AddTransceiverFromKind only accepts one RtpTransceiverInit")
	} else if len(init) == 1 {
		direction = init[0].Direction
	}

	switch direction {
	case RTPTransceiverDirectionSendrecv:
		receiver, err := pc.api.NewRTPReceiver(kind, pc.dtlsTransport)
		if err != nil {
			return nil, err
		}

		codecs := pc.api.mediaEngine.GetCodecsByKind(kind)
		if len(codecs) == 0 {
			return nil, fmt.Errorf("no %s codecs found", kind.String())
		}

		track, err := pc.NewTrack(codecs[0].PayloadType, mathRand.Uint32(), util.RandSeq(trackDefaultIDLength), util.RandSeq(trackDefaultLabelLength))
		if err != nil {
			return nil, err
		}

		sender, err := pc.api.NewRTPSender(track, pc.dtlsTransport)
		if err != nil {
			return nil, err
		}

		return pc.newRTPTransceiver(
			receiver,
			sender,
			RTPTransceiverDirectionSendrecv,
			kind,
		), nil

	case RTPTransceiverDirectionRecvonly:
		receiver, err := pc.api.NewRTPReceiver(kind, pc.dtlsTransport)
		if err != nil {
			return nil, err
		}

		return pc.newRTPTransceiver(
			receiver,
			nil,
			RTPTransceiverDirectionRecvonly,
			kind,
		), nil
	default:
		return nil, fmt.Errorf("AddTransceiverFromKind currently only supports recvonly and sendrecv")
	}
}

// AddTransceiverFromTrack Creates a new send only transceiver and add it to the set of
func (pc *PeerConnection) AddTransceiverFromTrack(track *Track, init ...RtpTransceiverInit) (*RTPTransceiver, error) {
	direction := RTPTransceiverDirectionSendrecv
	if len(init) > 1 {
		return nil, fmt.Errorf("AddTransceiverFromTrack only accepts one RtpTransceiverInit")
	} else if len(init) == 1 {
		direction = init[0].Direction
	}

	switch direction {
	case RTPTransceiverDirectionSendrecv:
		receiver, err := pc.api.NewRTPReceiver(track.Kind(), pc.dtlsTransport)
		if err != nil {
			return nil, err
		}

		sender, err := pc.api.NewRTPSender(track, pc.dtlsTransport)
		if err != nil {
			return nil, err
		}

		return pc.newRTPTransceiver(
			receiver,
			sender,
			RTPTransceiverDirectionSendrecv,
			track.Kind(),
		), nil

	case RTPTransceiverDirectionSendonly:
		sender, err := pc.api.NewRTPSender(track, pc.dtlsTransport)
		if err != nil {
			return nil, err
		}

		return pc.newRTPTransceiver(
			nil,
			sender,
			RTPTransceiverDirectionSendonly,
			track.Kind(),
		), nil
	default:
		return nil, fmt.Errorf("AddTransceiverFromTrack currently only supports sendonly and sendrecv")
	}
}



// SetIdentityProvider is used to configure an identity provider to generate identity assertions
func (pc *PeerConnection) SetIdentityProvider(provider string) error {
	return fmt.Errorf("TODO SetIdentityProvider")
}

// WriteRTCP sends a user provided RTCP packet to the connected peer
// If no peer is connected the packet is discarded
func (pc *PeerConnection) WriteRTCP(pkts []rtcp.Packet) error {
	raw, err := rtcp.Marshal(pkts)
	if err != nil {
		return err
	}

	srtcpSession, err := pc.dtlsTransport.getSRTCPSession()
	if err != nil {
		return nil
	}

	writeStream, err := srtcpSession.OpenWriteStream()
	if err != nil {
		return fmt.Errorf("WriteRTCP failed to open WriteStream: %v", err)
	}

	if _, err := writeStream.Write(raw); err != nil {
		return err
	}
	return nil
}


func (pc *PeerConnection) addFingerprint(d *sdp.SessionDescription) error {
	// pion/webrtc#753
	fingerprints, err := pc.configuration.Certificates[0].GetFingerprints()
	if err != nil {
		return err
	}
	for _, fingerprint := range fingerprints {
		d.WithFingerprint(fingerprint.Algorithm, strings.ToUpper(fingerprint.Value))
	}
	return nil
}

// NewTrack Creates a new Track
func (pc *PeerConnection) NewTrack(payloadType uint8, ssrc uint32, id, label string) (*Track, error) {
	codec, err := pc.api.mediaEngine.getCodec(payloadType)
	if err != nil {
		return nil, err
	} else if codec.Payloader == nil {
		return nil, fmt.Errorf("codec payloader not set")
	}

	return NewTrack(payloadType, ssrc, id, label, codec)
}

func (pc *PeerConnection) newRTPTransceiver(
	receiver *RTPReceiver,
	sender *RTPSender,
	direction RTPTransceiverDirection,
	kind RTPCodecType,
) *RTPTransceiver {
	t := &RTPTransceiver{
		Receiver:  receiver,
		Sender:    sender,
		Direction: direction,
		kind:      kind,
	}
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.rtpTransceivers = append(pc.rtpTransceivers, t)
	return t
}


// CurrentRemoteDescription represents the last remote description that was
// successfully negotiated the last time the PeerConnection transitioned
// into the stable state plus any remote candidates that have been supplied
// via AddICECandidate() since the offer or answer was created.
func (pc *PeerConnection) CurrentRemoteDescription() *SessionDescription {
	return pc.currentRemoteDescription
}

// PendingRemoteDescription represents a remote description that is in the
// process of being negotiated, complete with any remote candidates that
// have been supplied via AddICECandidate() since the offer or answer was
// created. If the PeerConnection is in the stable state, the value is
// null.
func (pc *PeerConnection) PendingRemoteDescription() *SessionDescription {
	return pc.pendingRemoteDescription
}

// SignalingState attribute returns the signaling state of the
// PeerConnection instance.
func (pc *PeerConnection) SignalingState() SignalingState {
	return pc.signalingState
}

// ICEGatheringState attribute returns the ICE gathering state of the
// PeerConnection instance.
func (pc *PeerConnection) ICEGatheringState() ICEGatheringState {
	switch pc.iceGatherer.State() {
	case ICEGathererStateNew:
		return ICEGatheringStateNew
	case ICEGathererStateGathering:
		return ICEGatheringStateGathering
	default:
		return ICEGatheringStateComplete
	}
}
