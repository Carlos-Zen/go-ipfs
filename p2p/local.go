package p2p

import (
	"context"
	"time"

	manet "gx/ipfs/QmRK2LxanhK2gZq6k6R7vk5ZoYZk8ULSSTB7FzDsMUX6CB/go-multiaddr-net"
	ma "gx/ipfs/QmWWQ2Txc2c6tqjsBpzg5Ar652cHPGNsQQp2SejkNmkUMb/go-multiaddr"
	net "gx/ipfs/QmXoz9o2PT3tEzf7hicegwex5UgVP54n3k82K7jrWFyN86/go-libp2p-net"
	protocol "gx/ipfs/QmZNkThpqfVXs9GNbexPrfBbXSLNYeKrE7jwFM2oqHbyqN/go-libp2p-protocol"
	peer "gx/ipfs/QmcJukH2sAFjY3HdBKq35WDzWoL3UUu2gt9wdfqZTUyM74/go-libp2p-peer"
	pstore "gx/ipfs/QmdeiKhUy1TVGBaKxt7y1QmBDLBdisSrLJ1x58Eoj4PXUh/go-libp2p-peerstore"
)

// localListener manet streams and proxies them to libp2p services
type localListener struct {
	ctx context.Context

	p2p *P2P
	id  peer.ID

	proto protocol.ID
	laddr ma.Multiaddr
	peer  peer.ID

	listener manet.Listener
}

// ForwardLocal creates new P2P stream to a remote listener
func (p2p *P2P) ForwardLocal(ctx context.Context, peer peer.ID, proto string, bindAddr ma.Multiaddr) (Listener, error) {
	listener := &localListener{
		ctx: ctx,

		p2p: p2p,
		id:  p2p.identity,

		proto: protocol.ID(proto),
		laddr: bindAddr,
		peer:  peer,
	}

	if err := p2p.Listeners.lock(listener); err != nil {
		return nil, err
	}

	maListener, err := manet.Listen(bindAddr)
	if err != nil {
		p2p.Listeners.unlock()
		return nil, err
	}

	listener.listener = maListener

	p2p.Listeners.Register(listener)
	go listener.acceptConns()

	return listener, nil
}

func (l *localListener) dial() (net.Stream, error) {
	ctx, cancel := context.WithTimeout(l.ctx, time.Second*30) //TODO: configurable?
	defer cancel()

	err := l.p2p.peerHost.Connect(ctx, pstore.PeerInfo{ID: l.peer})
	if err != nil {
		return nil, err
	}

	return l.p2p.peerHost.NewStream(l.ctx, l.peer, l.proto)
}

func (l *localListener) acceptConns() {
	for {
		local, err := l.listener.Accept()
		if err != nil {
			return
		}

		remote, err := l.dial()
		if err != nil {
			local.Close()
			return
		}

		tgt, err := ma.NewMultiaddr(l.TargetAddress())
		if err != nil {
			local.Close()
			return
		}

		stream := &Stream{
			Protocol: l.proto,

			OriginAddr: local.RemoteMultiaddr(),
			TargetAddr: tgt,

			Local:  local,
			Remote: remote,

			Registry: l.p2p.Streams,
		}

		l.p2p.Streams.Register(stream)
		stream.startStreaming()
	}
}

func (l *localListener) Close() error {
	l.listener.Close()
	l.p2p.Listeners.Deregister(getListenerKey(l))
	return nil
}

func (l *localListener) Protocol() string {
	return string(l.proto)
}

func (l *localListener) ListenAddress() string {
	return l.laddr.String()
}

func (l *localListener) TargetAddress() string {
	return "/ipfs/" + l.peer.Pretty()
}