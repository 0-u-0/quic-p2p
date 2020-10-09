package webrtc

import (
	"fmt"
	"sync"
	"time"

	"github.com/pion/ice"
)

// A Stats object contains a set of statistics copies out of a monitored component
// of the WebRTC stack at a specific time.
type Stats interface{}

// StatsType indicates the type of the object that a Stats object represents.
type StatsType string

const (

	// StatsTypeTransport is used by TransportStats.
	StatsTypeTransport StatsType = "transport"

	// StatsTypeCandidatePair is used by ICECandidatePairStats.
	StatsTypeCandidatePair StatsType = "candidate-pair"

	// StatsTypeLocalCandidate is used by ICECandidateStats for the local candidate.
	StatsTypeLocalCandidate StatsType = "local-candidate"

	// StatsTypeRemoteCandidate is used by ICECandidateStats for the remote candidate.
	StatsTypeRemoteCandidate StatsType = "remote-candidate"


)

// StatsTimestamp is a timestamp represented by the floating point number of
// milliseconds since the epoch.
type StatsTimestamp float64

// Time returns the time.Time represented by this timestamp.
func (s StatsTimestamp) Time() time.Time {
	millis := float64(s)
	nanos := int64(millis * float64(time.Millisecond))

	return time.Unix(0, nanos).UTC()
}

func statsTimestampFrom(t time.Time) StatsTimestamp {
	return StatsTimestamp(t.UnixNano() / int64(time.Millisecond))
}

// StatsReport collects Stats objects indexed by their ID.
type StatsReport map[string]Stats

type statsReportCollector struct {
	collectingGroup sync.WaitGroup
	report          StatsReport
	mux             sync.Mutex
}

func (src *statsReportCollector) Collecting() {
	src.collectingGroup.Add(1)
}

func (src *statsReportCollector) Collect(id string, stats Stats) {
	src.mux.Lock()
	defer src.mux.Unlock()

	src.report[id] = stats
	src.collectingGroup.Done()
}

func (src *statsReportCollector) Done() {
	src.collectingGroup.Done()
}

func (src *statsReportCollector) Ready() StatsReport {
	src.collectingGroup.Wait()
	src.mux.Lock()
	defer src.mux.Unlock()
	return src.report
}

// TransportStats contains transport statistics related to the PeerConnection object.
type TransportStats struct {
	// Timestamp is the timestamp associated with this object.
	Timestamp StatsTimestamp `json:"timestamp"`

	// Type is the object's StatsType
	Type StatsType `json:"type"`

	// ID is a unique id that is associated with the component inspected to produce
	// this Stats object. Two Stats objects will have the same ID if they were produced
	// by inspecting the same underlying object.
	ID string `json:"id"`

	// PacketsSent represents the total number of packets sent over this transport.
	PacketsSent uint32 `json:"packetsSent"`

	// PacketsReceived represents the total number of packets received on this transport.
	PacketsReceived uint32 `json:"packetsReceived"`

	// BytesSent represents the total number of payload bytes sent on this PeerConnection
	// not including headers or padding.
	BytesSent uint64 `json:"bytesSent"`

	// BytesReceived represents the total number of bytes received on this PeerConnection
	// not including headers or padding.
	BytesReceived uint64 `json:"bytesReceived"`

	// RTCPTransportStatsID is the ID of the transport that gives stats for the RTCP
	// component If RTP and RTCP are not multiplexed and this record has only
	// the RTP component stats.
	RTCPTransportStatsID string `json:"rtcpTransportStatsId"`

	// ICERole is set to the current value of the "role" attribute of the underlying
	// DTLSTransport's "transport".
	ICERole ICERole `json:"iceRole"`

	// DTLSState is set to the current value of the "state" attribute of the underlying DTLSTransport.
	DTLSState DTLSTransportState `json:"dtlsState"`

	// SelectedCandidatePairID is a unique identifier that is associated to the object
	// that was inspected to produce the ICECandidatePairStats associated with this transport.
	SelectedCandidatePairID string `json:"selectedCandidatePairId"`

	// LocalCertificateID is the ID of the CertificateStats for the local certificate.
	// Present only if DTLS is negotiated.
	LocalCertificateID string `json:"localCertificateId"`

	// LocalCertificateID is the ID of the CertificateStats for the remote certificate.
	// Present only if DTLS is negotiated.
	RemoteCertificateID string `json:"remoteCertificateId"`

	// DTLSCipher is the descriptive name of the cipher suite used for the DTLS transport,
	// as defined in the "Description" column of the IANA cipher suite registry.
	DTLSCipher string `json:"dtlsCipher"`

	// SRTPCipher is the descriptive name of the protection profile used for the SRTP
	// transport, as defined in the "Profile" column of the IANA DTLS-SRTP protection
	// profile registry.
	SRTPCipher string `json:"srtpCipher"`
}

// StatsICECandidatePairState is the state of an ICE candidate pair used in the
// ICECandidatePairStats object.
type StatsICECandidatePairState string

func toStatsICECandidatePairState(state ice.CandidatePairState) (StatsICECandidatePairState, error) {
	switch state {
	case ice.CandidatePairStateWaiting:
		return StatsICECandidatePairStateWaiting, nil
	case ice.CandidatePairStateInProgress:
		return StatsICECandidatePairStateInProgress, nil
	case ice.CandidatePairStateFailed:
		return StatsICECandidatePairStateFailed, nil
	case ice.CandidatePairStateSucceeded:
		return StatsICECandidatePairStateSucceeded, nil
	default:
		// NOTE: this should never happen[tm]
		err := fmt.Errorf(
			"cannot convert to StatsICECandidatePairStateSucceeded invalid ice candidate state: %s",
			state.String())
		return StatsICECandidatePairState(Unknown), err
	}
}

const (
	// StatsICECandidatePairStateFrozen means a check for this pair hasn't been
	// performed, and it can't yet be performed until some other check succeeds,
	// allowing this pair to unfreeze and move into the Waiting state.
	StatsICECandidatePairStateFrozen StatsICECandidatePairState = "frozen"

	// StatsICECandidatePairStateWaiting means a check has not been performed for
	// this pair, and can be performed as soon as it is the highest-priority Waiting
	// pair on the check list.
	StatsICECandidatePairStateWaiting StatsICECandidatePairState = "waiting"

	// StatsICECandidatePairStateInProgress means a check has been sent for this pair,
	// but the transaction is in progress.
	StatsICECandidatePairStateInProgress StatsICECandidatePairState = "in-progress"

	// StatsICECandidatePairStateFailed means a check for this pair was already done
	// and failed, either never producing any response or producing an unrecoverable
	// failure response.
	StatsICECandidatePairStateFailed StatsICECandidatePairState = "failed"

	// StatsICECandidatePairStateSucceeded means a check for this pair was already
	// done and produced a successful result.
	StatsICECandidatePairStateSucceeded StatsICECandidatePairState = "succeeded"
)

// ICECandidatePairStats contains ICE candidate pair statistics related
// to the ICETransport objects.
type ICECandidatePairStats struct {
	// Timestamp is the timestamp associated with this object.
	Timestamp StatsTimestamp `json:"timestamp"`

	// Type is the object's StatsType
	Type StatsType `json:"type"`

	// ID is a unique id that is associated with the component inspected to produce
	// this Stats object. Two Stats objects will have the same ID if they were produced
	// by inspecting the same underlying object.
	ID string `json:"id"`

	// TransportID is a unique identifier that is associated to the object that
	// was inspected to produce the TransportStats associated with this candidate pair.
	TransportID string `json:"transportId"`

	// LocalCandidateID is a unique identifier that is associated to the object
	// that was inspected to produce the ICECandidateStats for the local candidate
	// associated with this candidate pair.
	LocalCandidateID string `json:"localCandidateId"`

	// RemoteCandidateID is a unique identifier that is associated to the object
	// that was inspected to produce the ICECandidateStats for the remote candidate
	// associated with this candidate pair.
	RemoteCandidateID string `json:"remoteCandidateId"`

	// State represents the state of the checklist for the local and remote
	// candidates in a pair.
	State StatsICECandidatePairState `json:"state"`

	// Nominated is true when this valid pair that should be used for media
	// if it is the highest-priority one amongst those whose nominated flag is set
	Nominated bool `json:"nominated"`

	// PacketsSent represents the total number of packets sent on this candidate pair.
	PacketsSent uint32 `json:"packetsSent"`

	// PacketsReceived represents the total number of packets received on this candidate pair.
	PacketsReceived uint32 `json:"packetsReceived"`

	// BytesSent represents the total number of payload bytes sent on this candidate pair
	// not including headers or padding.
	BytesSent uint64 `json:"bytesSent"`

	// BytesReceived represents the total number of payload bytes received on this candidate pair
	// not including headers or padding.
	BytesReceived uint64 `json:"bytesReceived"`

	// LastPacketSentTimestamp represents the timestamp at which the last packet was
	// sent on this particular candidate pair, excluding STUN packets.
	LastPacketSentTimestamp StatsTimestamp `json:"lastPacketSentTimestamp"`

	// LastPacketReceivedTimestamp represents the timestamp at which the last packet
	// was received on this particular candidate pair, excluding STUN packets.
	LastPacketReceivedTimestamp StatsTimestamp `json:"lastPacketReceivedTimestamp"`

	// FirstRequestTimestamp represents the timestamp at which the first STUN request
	// was sent on this particular candidate pair.
	FirstRequestTimestamp StatsTimestamp `json:"firstRequestTimestamp"`

	// LastRequestTimestamp represents the timestamp at which the last STUN request
	// was sent on this particular candidate pair. The average interval between two
	// consecutive connectivity checks sent can be calculated with
	// (LastRequestTimestamp - FirstRequestTimestamp) / RequestsSent.
	LastRequestTimestamp StatsTimestamp `json:"lastRequestTimestamp"`

	// LastResponseTimestamp represents the timestamp at which the last STUN response
	// was received on this particular candidate pair.
	LastResponseTimestamp StatsTimestamp `json:"lastResponseTimestamp"`

	// TotalRoundTripTime represents the sum of all round trip time measurements
	// in seconds since the beginning of the session, based on STUN connectivity
	// check responses (ResponsesReceived), including those that reply to requests
	// that are sent in order to verify consent. The average round trip time can
	// be computed from TotalRoundTripTime by dividing it by ResponsesReceived.
	TotalRoundTripTime float64 `json:"totalRoundTripTime"`

	// CurrentRoundTripTime represents the latest round trip time measured in seconds,
	// computed from both STUN connectivity checks, including those that are sent
	// for consent verification.
	CurrentRoundTripTime float64 `json:"currentRoundTripTime"`

	// AvailableOutgoingBitrate is calculated by the underlying congestion control
	// by combining the available bitrate for all the outgoing RTP streams using
	// this candidate pair. The bitrate measurement does not count the size of the
	// IP or other transport layers like TCP or UDP. It is similar to the TIAS defined
	// in RFC 3890, i.e., it is measured in bits per second and the bitrate is calculated
	// over a 1 second window.
	AvailableOutgoingBitrate float64 `json:"availableOutgoingBitrate"`

	// AvailableIncomingBitrate is calculated by the underlying congestion control
	// by combining the available bitrate for all the incoming RTP streams using
	// this candidate pair. The bitrate measurement does not count the size of the
	// IP or other transport layers like TCP or UDP. It is similar to the TIAS defined
	// in  RFC 3890, i.e., it is measured in bits per second and the bitrate is
	// calculated over a 1 second window.
	AvailableIncomingBitrate float64 `json:"availableIncomingBitrate"`

	// CircuitBreakerTriggerCount represents the number of times the circuit breaker
	// is triggered for this particular 5-tuple, ceasing transmission.
	CircuitBreakerTriggerCount uint32 `json:"circuitBreakerTriggerCount"`

	// RequestsReceived represents the total number of connectivity check requests
	// received (including retransmissions). It is impossible for the receiver to
	// tell whether the request was sent in order to check connectivity or check
	// consent, so all connectivity checks requests are counted here.
	RequestsReceived uint64 `json:"requestsReceived"`

	// RequestsSent represents the total number of connectivity check requests
	// sent (not including retransmissions).
	RequestsSent uint64 `json:"requestsSent"`

	// ResponsesReceived represents the total number of connectivity check responses received.
	ResponsesReceived uint64 `json:"responsesReceived"`

	// ResponsesSent represents the total number of connectivity check responses sent.
	// Since we cannot distinguish connectivity check requests and consent requests,
	// all responses are counted.
	ResponsesSent uint64 `json:"responsesSent"`

	// RetransmissionsReceived represents the total number of connectivity check
	// request retransmissions received.
	RetransmissionsReceived uint64 `json:"retransmissionsReceived"`

	// RetransmissionsSent represents the total number of connectivity check
	// request retransmissions sent.
	RetransmissionsSent uint64 `json:"retransmissionsSent"`

	// ConsentRequestsSent represents the total number of consent requests sent.
	ConsentRequestsSent uint64 `json:"consentRequestsSent"`

	// ConsentExpiredTimestamp represents the timestamp at which the latest valid
	// STUN binding response expired.
	ConsentExpiredTimestamp StatsTimestamp `json:"consentExpiredTimestamp"`
}

// ICECandidateStats contains ICE candidate statistics related to the ICETransport objects.
type ICECandidateStats struct {
	// Timestamp is the timestamp associated with this object.
	Timestamp StatsTimestamp `json:"timestamp"`

	// Type is the object's StatsType
	Type StatsType `json:"type"`

	// ID is a unique id that is associated with the component inspected to produce
	// this Stats object. Two Stats objects will have the same ID if they were produced
	// by inspecting the same underlying object.
	ID string `json:"id"`

	// TransportID is a unique identifier that is associated to the object that
	// was inspected to produce the TransportStats associated with this candidate.
	TransportID string `json:"transportId"`

	// NetworkType represents the type of network interface used by the base of a
	// local candidate (the address the ICE agent sends from). Only present for
	// local candidates; it's not possible to know what type of network interface
	// a remote candidate is using.
	//
	// Note:
	// This stat only tells you about the network interface used by the first "hop";
	// it's possible that a connection will be bottlenecked by another type of network.
	// For example, when using Wi-Fi tethering, the networkType of the relevant candidate
	// would be "wifi", even when the next hop is over a cellular connection.
	NetworkType NetworkType `json:"networkType"`

	// IP is the IP address of the candidate, allowing for IPv4 addresses and
	// IPv6 addresses, but fully qualified domain names (FQDNs) are not allowed.
	IP string `json:"ip"`

	// Port is the port number of the candidate.
	Port int32 `json:"port"`

	// Protocol is one of udp and tcp.
	Protocol string `json:"protocol"`

	// CandidateType is the "Type" field of the ICECandidate.
	CandidateType ICECandidateType `json:"candidateType"`

	// Priority is the "Priority" field of the ICECandidate.
	Priority int32 `json:"priority"`

	// URL is the URL of the TURN or STUN server indicated in the that translated
	// this IP address. It is the URL address surfaced in an PeerConnectionICEEvent.
	URL string `json:"url"`

	// RelayProtocol is the protocol used by the endpoint to communicate with the
	// TURN server. This is only present for local candidates. Valid values for
	// the TURN URL protocol is one of udp, tcp, or tls.
	RelayProtocol string `json:"relayProtocol"`

	// Deleted is true if the candidate has been deleted/freed. For host candidates,
	// this means that any network resources (typically a socket) associated with the
	// candidate have been released. For TURN candidates, this means the TURN allocation
	// is no longer active.
	//
	// Only defined for local candidates. For remote candidates, this property is not applicable.
	Deleted bool `json:"deleted"`
}
