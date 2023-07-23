// Copyright (C) 2023  mieru authors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>

package protocolv2

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/enfein/mieru/pkg/cipher"
	"github.com/enfein/mieru/pkg/log"
	"github.com/enfein/mieru/pkg/netutil"
	"github.com/enfein/mieru/pkg/replay"
	"github.com/enfein/mieru/pkg/rng"
	"github.com/enfein/mieru/pkg/stderror"
)

type TCPUnderlay struct {
	baseUnderlay

	conn *net.TCPConn

	send cipher.BlockCipher
	recv cipher.BlockCipher

	// Candidates are block ciphers that can be used to encrypt or decrypt data.
	// When isClient is true, there must be exactly 1 element in the slice.
	candidates []cipher.BlockCipher

	// sendMutex is used when write data to the connection.
	sendMutex sync.Mutex
}

var _ Underlay = &TCPUnderlay{}

var tcpReplayCache = replay.NewCache(16*1024*1024, 2*time.Minute)

// NewTCPUnderlay connects to the remote address "raddr" on the network "tcp"
// with packet encryption. If "laddr" is empty, an automatic address is used.
// "block" is the block encryption algorithm to encrypt packets.
func NewTCPUnderlay(ctx context.Context, network, laddr, raddr string, mtu int, block cipher.BlockCipher) (*TCPUnderlay, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, fmt.Errorf("network %s is not supported for TCP underlay", network)
	}
	if block.IsStateless() {
		return nil, fmt.Errorf("TCP block cipher must not be stateless")
	}
	dialer := net.Dialer{}
	if laddr != "" {
		tcpLocalAddr, err := net.ResolveTCPAddr(network, laddr)
		if err != nil {
			return nil, fmt.Errorf("net.ResolveTCPAddr() failed: %w", err)
		}
		dialer.LocalAddr = tcpLocalAddr
	}

	conn, err := dialer.DialContext(ctx, network, raddr)
	if err != nil {
		return nil, fmt.Errorf("DialContext() failed: %w", err)
	}
	log.Debugf("Created new client TCP underlay [%v - %v]", conn.LocalAddr(), conn.RemoteAddr())
	return &TCPUnderlay{
		baseUnderlay: *newBaseUnderlay(true, mtu),
		conn:         conn.(*net.TCPConn),
		candidates:   []cipher.BlockCipher{block},
	}, nil
}

func (t *TCPUnderlay) String() string {
	if t.conn == nil {
		return "TCPUnderlay{}"
	}
	return fmt.Sprintf("TCPUnderlay{%v - %v}", t.conn.LocalAddr(), t.conn.RemoteAddr())
}

func (t *TCPUnderlay) Close() error {
	select {
	case <-t.done:
		return nil
	default:
	}

	log.Debugf("Closing %v", t)
	t.baseUnderlay.Close()
	return t.conn.Close()
}

func (t *TCPUnderlay) Addr() net.Addr {
	return t.LocalAddr()
}

func (t *TCPUnderlay) IPVersion() netutil.IPVersion {
	if t.conn == nil {
		return netutil.IPVersionUnknown
	}
	return netutil.GetIPVersion(t.conn.LocalAddr().String())
}

func (t *TCPUnderlay) TransportProtocol() netutil.TransportProtocol {
	return netutil.TCPTransport
}

func (t *TCPUnderlay) LocalAddr() net.Addr {
	return t.conn.LocalAddr()
}

func (t *TCPUnderlay) RemoteAddr() net.Addr {
	return t.conn.RemoteAddr()
}

func (t *TCPUnderlay) AddSession(s *Session) error {
	if err := t.baseUnderlay.AddSession(s); err != nil {
		return err
	}
	s.conn = t // override base underlay
	close(s.ready)
	log.Debugf("Adding session %d to %v", s.id, t)

	s.wg.Add(2)
	go func() {
		if err := s.runInputLoop(context.Background()); err != nil {
			log.Debugf("%v runInputLoop(): %v", s, err)
		}
		s.wg.Done()
	}()
	go func() {
		if err := s.runOutputLoop(context.Background()); err != nil {
			log.Debugf("%v runOutputLoop(): %v", s, err)
		}
		s.wg.Done()
	}()
	return nil
}

func (t *TCPUnderlay) RemoveSession(s *Session) error {
	err := t.baseUnderlay.RemoveSession(s)
	if len(t.baseUnderlay.sessionMap) == 0 {
		t.Close()
	}
	return err
}

func (t *TCPUnderlay) RunEventLoop(ctx context.Context) error {
	if t.conn == nil {
		return stderror.ErrNullPointer
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.done:
			return nil
		default:
		}
		seg, err := t.readOneSegment()
		if err != nil {
			return fmt.Errorf("readOneSegment() failed: %w", err)
		}
		if log.IsLevelEnabled(log.TraceLevel) {
			log.Tracef("%v received one segment: protocol = %d, payload size = %d", t, seg.metadata.Protocol(), len(seg.payload))
		}
		if isSessionProtocol(seg.metadata.Protocol()) {
			switch seg.Protocol() {
			case openSessionRequest:
				if err := t.onOpenSessionRequest(seg); err != nil {
					return fmt.Errorf("onOpenSessionRequest() failed: %v", err)
				}
			case openSessionResponse:
				if err := t.onOpenSessionResponse(seg); err != nil {
					return fmt.Errorf("onOpenSessionResponse() failed: %v", err)
				}
			case closeSessionRequest, closeSessionResponse:
				if err := t.onCloseSession(seg); err != nil {
					return fmt.Errorf("onCloseSession() failed: %v", err)
				}
			case closeConnRequest:
				// Ignore. Expect to close TCP connection directly.
			case closeConnResponse:
				// Ignore. Expect to close TCP connection directly.
			default:
				// Ignore unknown protocol from a future version.
			}
		} else if isDataAckProtocol(seg.metadata.Protocol()) {
			das, _ := toDataAckStruct(seg.metadata)
			t.sessionLock.Lock()
			session, ok := t.sessionMap[das.sessionID]
			t.sessionLock.Unlock()
			if !ok {
				log.Debugf("Session %d is not registered to %s", das.sessionID, t.String())
				continue
			}
			session.recvChan <- seg
		}
	}
}

func (t *TCPUnderlay) onOpenSessionRequest(seg *segment) error {
	if t.isClient {
		return stderror.ErrInvalidOperation
	}

	// Create a new session.
	sessionID := seg.metadata.(*sessionStruct).sessionID
	if sessionID == 0 {
		// 0 is reserved and can't be used.
		return fmt.Errorf("reserved session ID %d is used", sessionID)
	}
	t.sessionLock.Lock()
	_, found := t.sessionMap[sessionID]
	t.sessionLock.Unlock()
	if found {
		return fmt.Errorf("session ID %d is already used", sessionID)
	}
	session := NewSession(sessionID, t.isClient, t.MTU())
	t.AddSession(session)
	session.recvChan <- seg
	t.readySessions <- session
	return nil
}

func (t *TCPUnderlay) onOpenSessionResponse(seg *segment) error {
	if !t.isClient {
		return stderror.ErrInvalidOperation
	}

	sessionID := seg.metadata.(*sessionStruct).sessionID
	t.sessionLock.Lock()
	session, found := t.sessionMap[sessionID]
	t.sessionLock.Unlock()
	if !found {
		return fmt.Errorf("session ID %d is not found", sessionID)
	}
	session.recvChan <- seg
	return nil
}

func (t *TCPUnderlay) onCloseSession(seg *segment) error {
	ss := seg.metadata.(*sessionStruct)
	sessionID := ss.sessionID
	t.sessionLock.Lock()
	session, found := t.sessionMap[sessionID]
	t.sessionLock.Unlock()
	if !found {
		return fmt.Errorf("session ID %d is not found", sessionID)
	}
	session.recvChan <- seg
	session.wg.Wait()
	t.RemoveSession(session)
	return nil
}

func (t *TCPUnderlay) readOneSegment() (*segment, error) {
	var firstRead bool
	var err error

	// Read encrypted metadata.
	readLen := metadataLength + cipher.DefaultOverhead
	if t.recv == nil {
		// In the first Read, also include nonce.
		firstRead = true
		readLen += cipher.DefaultNonceSize
	}
	encryptedMeta := make([]byte, readLen)
	if _, err := io.ReadFull(t.conn, encryptedMeta); err != nil {
		return nil, fmt.Errorf("metadata: read %d bytes from TCPUnderlay failed: %w", readLen, err)
	}
	if tcpReplayCache.IsDuplicate(encryptedMeta[:cipher.DefaultOverhead], replay.EmptyTag) {
		if firstRead {
			replay.NewSession.Add(1)
			return nil, fmt.Errorf("found possible replay attack in %v", t)
		} else {
			replay.KnownSession.Add(1)
		}
	}

	// Decrypt metadata.
	var decryptedMeta []byte
	if t.recv == nil && t.isClient {
		t.recv = t.candidates[0].Clone()
	}
	if t.recv == nil {
		var peerBlock cipher.BlockCipher
		peerBlock, decryptedMeta, err = cipher.SelectDecrypt(encryptedMeta, cipher.CloneBlockCiphers(t.candidates))
		if err != nil {
			return nil, fmt.Errorf("cipher.SelectDecrypt() failed: %w", err)
		}
		t.recv = peerBlock.Clone()
	} else {
		decryptedMeta, err = t.recv.Decrypt(encryptedMeta)
		if err != nil {
			return nil, fmt.Errorf("Decrypt() failed: %w", err)
		}
	}
	if len(decryptedMeta) != metadataLength {
		return nil, fmt.Errorf("decrypted metadata size %d is unexpected", len(decryptedMeta))
	}

	// Read payload and construct segment.
	p := decryptedMeta[0]
	if isSessionProtocol(p) {
		ss := &sessionStruct{}
		if err := ss.Unmarshal(decryptedMeta); err != nil {
			return nil, fmt.Errorf("Unmarshal() failed: %w", err)
		}
		return t.readSessionSegment(ss)
	} else if isDataAckProtocol(p) {
		das := &dataAckStruct{}
		if err := das.Unmarshal(decryptedMeta); err != nil {
			return nil, fmt.Errorf("Unmarshal() failed: %w", err)
		}
		return t.readDataAckSegment(das)
	}

	return nil, fmt.Errorf("unable to handle protocol %d", p)
}

func (t *TCPUnderlay) readSessionSegment(ss *sessionStruct) (*segment, error) {
	var decryptedPayload []byte
	var err error

	if ss.payloadLen > 0 {
		encryptedPayload := make([]byte, ss.payloadLen+cipher.DefaultOverhead)
		if _, err := io.ReadFull(t.conn, encryptedPayload); err != nil {
			return nil, fmt.Errorf("payload: read %d bytes from TCPUnderlay failed: %w", ss.payloadLen+cipher.DefaultOverhead, err)
		}
		if tcpReplayCache.IsDuplicate(encryptedPayload[:cipher.DefaultOverhead], replay.EmptyTag) {
			replay.KnownSession.Add(1)
		}
		decryptedPayload, err = t.recv.Decrypt(encryptedPayload)
		if err != nil {
			return nil, fmt.Errorf("Decrypt() failed: %w", err)
		}
	}
	if ss.suffixLen > 0 {
		padding := make([]byte, ss.suffixLen)
		if _, err := io.ReadFull(t.conn, padding); err != nil {
			return nil, fmt.Errorf("padding: read %d bytes from TCPUnderlay failed: %w", ss.suffixLen, err)
		}
	}

	return &segment{
		metadata: ss,
		payload:  decryptedPayload,
	}, nil
}

func (t *TCPUnderlay) readDataAckSegment(das *dataAckStruct) (*segment, error) {
	var decryptedPayload []byte
	var err error

	if das.prefixLen > 0 {
		padding1 := make([]byte, das.prefixLen)
		if _, err := io.ReadFull(t.conn, padding1); err != nil {
			return nil, fmt.Errorf("padding: read %d bytes from TCPUnderlay failed: %w", das.prefixLen, err)
		}
	}
	if das.payloadLen > 0 {
		encryptedPayload := make([]byte, das.payloadLen+cipher.DefaultOverhead)
		if _, err := io.ReadFull(t.conn, encryptedPayload); err != nil {
			return nil, fmt.Errorf("payload: read %d bytes from TCPUnderlay failed: %w", das.payloadLen+cipher.DefaultOverhead, err)
		}
		if tcpReplayCache.IsDuplicate(encryptedPayload[:cipher.DefaultOverhead], replay.EmptyTag) {
			replay.KnownSession.Add(1)
		}
		decryptedPayload, err = t.recv.Decrypt(encryptedPayload)
		if err != nil {
			return nil, fmt.Errorf("Decrypt() failed: %w", err)
		}
	}
	if das.suffixLen > 0 {
		padding2 := make([]byte, das.suffixLen)
		if _, err := io.ReadFull(t.conn, padding2); err != nil {
			return nil, fmt.Errorf("padding: read %d bytes from TCPUnderlay failed: %w", das.suffixLen, err)
		}
	}

	return &segment{
		metadata: das,
		payload:  decryptedPayload,
	}, nil
}

func (t *TCPUnderlay) writeOneSegment(seg *segment) error {
	if seg == nil {
		return stderror.ErrNullPointer
	}

	t.sendMutex.Lock()
	defer t.sendMutex.Unlock()

	if ss, ok := toSessionStruct(seg.metadata); ok {
		suffixLen := rng.Intn(255)
		ss.suffixLen = uint8(suffixLen)
		padding := newPadding(suffixLen)

		plaintextMetadata := ss.Marshal()
		if err := t.maybeInitSendBlockCipher(); err != nil {
			return fmt.Errorf("maybeInitSendBlockCipher() failed: %w", err)
		}
		encryptedMetadata, err := t.send.Encrypt(plaintextMetadata)
		if err != nil {
			return fmt.Errorf("Encrypt() failed: %w", err)
		}
		dataToSend := encryptedMetadata
		if len(seg.payload) > 0 {
			encryptedPayload, err := t.send.Encrypt(seg.payload)
			if err != nil {
				return fmt.Errorf("Encrypt() failed: %w", err)
			}
			dataToSend = append(dataToSend, encryptedPayload...)
		}
		dataToSend = append(dataToSend, padding...)
		if _, err := t.conn.Write(dataToSend); err != nil {
			return fmt.Errorf("Write() failed: %w", err)
		}
	} else if das, ok := toDataAckStruct(seg.metadata); ok {
		paddingLen1 := rng.Intn(255)
		paddingLen2 := rng.Intn(255)
		das.prefixLen = uint8(paddingLen1)
		das.suffixLen = uint8(paddingLen2)
		padding1 := newPadding(paddingLen1)
		padding2 := newPadding(paddingLen2)

		plaintextMetadata := das.Marshal()
		if err := t.maybeInitSendBlockCipher(); err != nil {
			return fmt.Errorf("maybeInitSendBlockCipher() failed: %w", err)
		}
		encryptedMetadata, err := t.send.Encrypt(plaintextMetadata)
		if err != nil {
			return fmt.Errorf("Encrypt() failed: %w", err)
		}
		dataToSend := append(encryptedMetadata, padding1...)
		if len(seg.payload) > 0 {
			encryptedPayload, err := t.send.Encrypt(seg.payload)
			if err != nil {
				return fmt.Errorf("Encrypt() failed: %w", err)
			}
			dataToSend = append(dataToSend, encryptedPayload...)
		}
		dataToSend = append(dataToSend, padding2...)
		if _, err := t.conn.Write(dataToSend); err != nil {
			return fmt.Errorf("Write() failed: %w", err)
		}
	} else {
		return stderror.ErrInvalidArgument
	}
	return nil
}

func (t *TCPUnderlay) maybeInitSendBlockCipher() error {
	if t.send != nil {
		return nil
	}
	if t.isClient {
		t.send = t.candidates[0].Clone()
	} else {
		if t.recv != nil {
			t.send = t.recv.Clone()
			t.send.SetImplicitNonceMode(false) // clear implicit nonce
			t.send.SetImplicitNonceMode(true)
		} else {
			return fmt.Errorf("recv cipher is nil")
		}
	}
	return nil
}
