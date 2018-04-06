package p2p

import (
	"fmt"
	"io"

	manet "gx/ipfs/QmRK2LxanhK2gZq6k6R7vk5ZoYZk8ULSSTB7FzDsMUX6CB/go-multiaddr-net"
	ma "gx/ipfs/QmWWQ2Txc2c6tqjsBpzg5Ar652cHPGNsQQp2SejkNmkUMb/go-multiaddr"
	net "gx/ipfs/QmXoz9o2PT3tEzf7hicegwex5UgVP54n3k82K7jrWFyN86/go-libp2p-net"
	peer "gx/ipfs/QmcJukH2sAFjY3HdBKq35WDzWoL3UUu2gt9wdfqZTUyM74/go-libp2p-peer"
)

// ListenerRegistry is a collection of local application proto listeners.
type ListenerRegistry struct {
	Listeners []Listener
}

// Register registers listenerInfo2 in this registry
func (c *ListenerRegistry) Register(listenerInfo Listener) {
	c.Listeners = append(c.Listeners, listenerInfo)
}

// Deregister removes p2p listener from this registry
func (c *ListenerRegistry) Deregister(proto string) error {
	foundAt := -1
	for i, a := range c.Listeners {
		if a.Protocol() == proto {
			foundAt = i
			break
		}
	}

	if foundAt != -1 {
		c.Listeners = append(c.Listeners[:foundAt], c.Listeners[foundAt+1:]...)
		return nil
	}

	return fmt.Errorf("failed to deregister proto %s", proto)
}

// StreamInfo holds information on active incoming and outgoing p2p streams.
type StreamInfo struct {
	HandlerID uint64

	Protocol string

	LocalPeer peer.ID
	LocalAddr ma.Multiaddr

	RemotePeer peer.ID
	RemoteAddr ma.Multiaddr

	Local  manet.Conn
	Remote net.Stream

	Registry *StreamRegistry
}

// Close closes stream endpoints and deregisters it
func (s *StreamInfo) Close() error {
	s.Local.Close()
	s.Remote.Close()
	s.Registry.Deregister(s.HandlerID)
	return nil
}

// Reset closes stream endpoints and deregisters it
func (s *StreamInfo) Reset() error {
	s.Local.Close()
	s.Remote.Reset()
	s.Registry.Deregister(s.HandlerID)
	return nil
}

func (s *StreamInfo) startStreaming() {
	go func() {
		_, err := io.Copy(s.Local, s.Remote)
		if err != nil {
			s.Reset()
		} else {
			s.Close()
		}
	}()

	go func() {
		_, err := io.Copy(s.Remote, s.Local)
		if err != nil {
			s.Reset()
		} else {
			s.Close()
		}
	}()
}

// StreamRegistry is a collection of active incoming and outgoing proto app streams.
type StreamRegistry struct {
	Streams []*StreamInfo

	nextID uint64
}

// Register registers a stream to the registry
func (c *StreamRegistry) Register(streamInfo *StreamInfo) {
	streamInfo.HandlerID = c.nextID
	c.Streams = append(c.Streams, streamInfo)
	c.nextID++
}

// Deregister deregisters stream from the registry
func (c *StreamRegistry) Deregister(handlerID uint64) {
	foundAt := -1
	for i, s := range c.Streams {
		if s.HandlerID == handlerID {
			foundAt = i
			break
		}
	}

	if foundAt != -1 {
		c.Streams = append(c.Streams[:foundAt], c.Streams[foundAt+1:]...)
	}
}
