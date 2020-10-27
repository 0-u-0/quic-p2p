package main

import (
	"fmt"
	"time"

	"github.com/0-u-0/quic-p2p/v3"
	"github.com/pion/quic"
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

	setting := &webrtc.SettingEngine{}
	// Create the ICE gatherer
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
		go offerReadLoop(stream)

		// Handle writing to the stream
		go offerWriteLoop(stream)
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

	iceRole := webrtc.ICERoleControlling


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

	var stream *quic.BidirectionalStream
	stream, err = qt.CreateBidirectionalStream()
	if err != nil {
		panic(err)
	}

	// Handle reading from the stream
	go offerReadLoop(stream)

	// Handle writing to the stream
	go offerWriteLoop(stream)

	select {}
}


// ReadLoop reads from the stream
func offerReadLoop(s *quic.BidirectionalStream) {
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
func offerWriteLoop(s *quic.BidirectionalStream) {
	for range time.NewTicker(5 * time.Second).C {
		message := "offer_send"
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
