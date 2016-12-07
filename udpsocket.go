package steam

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"time"
)

type udpSocket struct {
	conn net.Conn
}

func newUDPSocket(dial DialFn, addr string) (*udpSocket, error) {
	conn, err := dial("udp", addr)
	if err != nil {
		return nil, err
	}
	return &udpSocket{conn}, nil
}

func (s *udpSocket) close() {
	s.conn.Close()
}

func (s *udpSocket) send(payload []byte) error {
	n, err := s.conn.Write(payload)
	if err != nil {
		return err
	}
	if n != len(payload) {
		return fmt.Errorf("steam: could not send full udp request to %v", s.conn.RemoteAddr())
	}
	return nil
}

func (s *udpSocket) receivePacket() ([]byte, error) {
	if err := s.conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		return nil, err
	}
	buf := make([]byte, 1500)
	n, err := s.conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (s *udpSocket) receive() ([]byte, error) {
	buf, err := s.receivePacket()
	if err != nil {
		return nil, err
	}
	if buf[0] == 0xFE {
		// return nil, errors.New("steam: cannot handle split packets")
		return s.receiveMultiByte(buf)
	}
	return buf[4:], nil
}

func (s *udpSocket) receiveMultiByte(firstPacket []byte) ([]byte, error) {
	assembledPacketBuf := new(bytes.Buffer)

	packetBuf := bytes.NewBuffer(firstPacket[4:])

	id := toInt(readLong(packetBuf))
	totalPackets := toInt(readByte(packetBuf))
	toInt(readByte(packetBuf))
	size := toInt(readShort(packetBuf))

	if id&0xFF == 1 {
		return nil, errors.New("steam: cannot handle compressed packets")
	}

	// Write the data of the first packet.
	packetBytes := make([]byte, size)
	numBytes, _ := packetBuf.Read(packetBytes)
	log.Println("wrote bytes:", numBytes)
	assembledPacketBuf.Write(packetBytes[:numBytes])

	countedPackets := 1
	for {
		packet, err := s.receivePacket()
		if err != nil {
			return nil, err
		}
		packetBuf = bytes.NewBuffer(packet)

		// Skip header.
		readLong(packetBuf)

		// Read ID
		if id != toInt(readLong(packetBuf)) {
			return nil, errors.New("steam: response id invalid")
		}

		// Skip total packets.
		readByte(packetBuf)

		// Skip packet number.
		readByte(packetBuf)

		// Make sure packet is not compressed.
		if id&0xFF == 1 {
			return nil, errors.New("steam: cannot handle compressed packets")
		}

		// Read size
		size = toInt(readShort(packetBuf))

		// Write payload to assembled packet.
		packetBytes := make([]byte, size)
		numBytes, _ := packetBuf.Read(packetBytes)
		log.Println("wrote bytes:", numBytes)
		assembledPacketBuf.Write(packetBytes[:numBytes])

		// Increase the packet counter.
		countedPackets++

		if countedPackets >= totalPackets {
			break
		}
	}

	return assembledPacketBuf.Bytes(), nil
}
