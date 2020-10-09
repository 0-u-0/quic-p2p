// +build !js

package webrtc

import "github.com/pion/sdp/v2"


func newICECandidateFromSDP(c sdp.ICECandidate) (ICECandidate, error) {
	typ, err := NewICECandidateType(c.Typ)
	if err != nil {
		return ICECandidate{}, err
	}
	protocol, err := NewICEProtocol(c.Protocol)
	if err != nil {
		return ICECandidate{}, err
	}
	return ICECandidate{
		Foundation:     c.Foundation,
		Priority:       c.Priority,
		Address:        c.Address,
		Protocol:       protocol,
		Port:           c.Port,
		Component:      c.Component,
		Typ:            typ,
		RelatedAddress: c.RelatedAddress,
		RelatedPort:    c.RelatedPort,
	}, nil
}
