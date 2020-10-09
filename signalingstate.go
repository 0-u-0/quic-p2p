package webrtc

type stateChangeOp int

const (
	stateChangeOpSetLocal stateChangeOp = iota + 1
	stateChangeOpSetRemote
)

func (op stateChangeOp) String() string {
	switch op {
	case stateChangeOpSetLocal:
		return "SetLocal"
	case stateChangeOpSetRemote:
		return "SetRemote"
	default:
		return "Unknown State Change Operation"
	}
}

// SignalingState indicates the signaling state of the offer/answer process.
type SignalingState int

const (
	// SignalingStateStable indicates there is no offer/answer exchange in
	// progress. This is also the initial state, in which case the local and
	// remote descriptions are nil.
	SignalingStateStable SignalingState = iota + 1

	// SignalingStateHaveLocalOffer indicates that a local description, of
	// type "offer", has been successfully applied.
	SignalingStateHaveLocalOffer

	// SignalingStateHaveRemoteOffer indicates that a remote description, of
	// type "offer", has been successfully applied.
	SignalingStateHaveRemoteOffer

	// SignalingStateHaveLocalPranswer indicates that a remote description
	// of type "offer" has been successfully applied and a local description
	// of type "pranswer" has been successfully applied.
	SignalingStateHaveLocalPranswer

	// SignalingStateHaveRemotePranswer indicates that a local description
	// of type "offer" has been successfully applied and a remote description
	// of type "pranswer" has been successfully applied.
	SignalingStateHaveRemotePranswer

	// SignalingStateClosed indicates The PeerConnection has been closed.
	SignalingStateClosed
)

// This is done this way because of a linter.
const (
	signalingStateStableStr             = "stable"
	signalingStateHaveLocalOfferStr     = "have-local-offer"
	signalingStateHaveRemoteOfferStr    = "have-remote-offer"
	signalingStateHaveLocalPranswerStr  = "have-local-pranswer"
	signalingStateHaveRemotePranswerStr = "have-remote-pranswer"
	signalingStateClosedStr             = "closed"
)

func newSignalingState(raw string) SignalingState {
	switch raw {
	case signalingStateStableStr:
		return SignalingStateStable
	case signalingStateHaveLocalOfferStr:
		return SignalingStateHaveLocalOffer
	case signalingStateHaveRemoteOfferStr:
		return SignalingStateHaveRemoteOffer
	case signalingStateHaveLocalPranswerStr:
		return SignalingStateHaveLocalPranswer
	case signalingStateHaveRemotePranswerStr:
		return SignalingStateHaveRemotePranswer
	case signalingStateClosedStr:
		return SignalingStateClosed
	default:
		return SignalingState(Unknown)
	}
}

func (t SignalingState) String() string {
	switch t {
	case SignalingStateStable:
		return signalingStateStableStr
	case SignalingStateHaveLocalOffer:
		return signalingStateHaveLocalOfferStr
	case SignalingStateHaveRemoteOffer:
		return signalingStateHaveRemoteOfferStr
	case SignalingStateHaveLocalPranswer:
		return signalingStateHaveLocalPranswerStr
	case SignalingStateHaveRemotePranswer:
		return signalingStateHaveRemotePranswerStr
	case SignalingStateClosed:
		return signalingStateClosedStr
	default:
		return ErrUnknownType.Error()
	}
}
