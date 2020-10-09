package main

import (
	"fmt"
	"time"

	"github.com/pion/quic"
	"github.com/pion/webrtc/v2"

)


func main() {

	// This example shows off the experimental implementation of webrtc-quic.

	// Everything below is the Pion WebRTC (ORTC) API! Thanks for using it ❤️.

	// Create an API object

	// Prepare ICE gathering options
	iceOptions := webrtc.ICEGatherOptions{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	// Create the ICE gatherer
	setting := &webrtc.SettingEngine{}

	gatherer, err := webrtc.NewICEGatherer(iceOptions, setting)
	if err != nil {
		panic(err)
	}

	// Construct the ICE transport
	ice := webrtc.NewICETransport(gatherer)

	// Construct the Quic transport
	qt, err := webrtc.NewQUICTransport(ice, nil)
	if err != nil {
		panic(err)
	}

	// Handle incoming streams
	qt.OnBidirectionalStream(func(stream *quic.BidirectionalStream) {
		fmt.Printf("New stream %d\n", stream.StreamID())

		// Handle reading from the stream
		go answerReadLoop(stream)

		// Handle writing to the stream
		go answerWriteLoop(stream)
	})

	// Gather candidates
	err = gatherer.Gather()
	if err != nil {
		panic(err)
	}

	iceCandidates, err := gatherer.GetLocalCandidates()
	if err != nil {
		panic(err)
	}

	iceParams, err := gatherer.GetLocalParameters()
	if err != nil {
		panic(err)
	}

	quicParams, err := qt.GetLocalParameters()
	if err != nil {
		panic(err)
	}

	s := Signal{
		ICECandidates:  iceCandidates,
		ICEParameters:  iceParams,
		QuicParameters: quicParams,
	}

	// Exchange the information
	fmt.Println(Encode(s))
	remoteSignal := Signal{}
	Decode(MustReadStdin(), &remoteSignal)

	iceRole := webrtc.ICERoleControlled

	err = ice.SetRemoteCandidates(remoteSignal.ICECandidates)
	if err != nil {
		panic(err)
	}

	// Start the ICE transport
	err = ice.Start(nil, remoteSignal.ICEParameters, &iceRole)
	if err != nil {
		panic(err)
	}

	// Start the Quic transport
	err = qt.Start(remoteSignal.QuicParameters)
	if err != nil {
		panic(err)
	}



	select {}
}

// ReadLoop reads from the stream
func answerReadLoop(s *quic.BidirectionalStream) {
	for {
		buffer := make([]byte, messageSize)
		params, err := s.ReadInto(buffer)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Message from stream '%d': %s\n", s.StreamID(), string(buffer[:params.Amount]))
	}
}

// WriteLoop writes to the stream
func answerWriteLoop(s *quic.BidirectionalStream) {
	for range time.NewTicker(5 * time.Second).C {
		message := "answer_send"
		fmt.Printf("Sending %s \n", message)

		data := quic.StreamWriteParameters{
			Data: []byte(message),
		}
		err := s.Write(data)
		if err != nil {
			panic(err)
		}
	}
}
