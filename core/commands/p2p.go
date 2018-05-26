package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	cmds "github.com/ipfs/go-ipfs/commands"
	core "github.com/ipfs/go-ipfs/core"
	p2p "github.com/ipfs/go-ipfs/p2p"

	ma "gx/ipfs/QmWWQ2Txc2c6tqjsBpzg5Ar652cHPGNsQQp2SejkNmkUMb/go-multiaddr"
	pstore "gx/ipfs/QmXauCuJzmzapetmC6W4TuDJLL1yFFrVzSHoWv8YdbmnxH/go-libp2p-peerstore"
	"gx/ipfs/QmceUdzxkimdYsgtX733uNgzf1DLHyBKN6ehGSp85ayppM/go-ipfs-cmdkit"
)

// P2PListenerInfoOutput is output type of ls command
type P2PListenerInfoOutput struct {
	Protocol      string
	ListenAddress string
	TargetAddress string
}

// P2PStreamInfoOutput is output type of streams command
type P2PStreamInfoOutput struct {
	HandlerID     string
	Protocol      string
	OriginAddress string
	TargetAddress string
}

// P2PLsOutput is output type of ls command
type P2PLsOutput struct {
	Listeners []P2PListenerInfoOutput
}

// P2PStreamsOutput is output type of streams command
type P2PStreamsOutput struct {
	Streams []P2PStreamInfoOutput
}

// P2PCmd is the 'ipfs p2p' command
var P2PCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Libp2p stream mounting.",
		ShortDescription: `
Create and use tunnels to remote peers over libp2p

Note: this command is experimental and subject to change as usecases and APIs
are refined`,
	},

	Subcommands: map[string]*cmds.Command{
		"stream": p2pStreamCmd,

		"forward": p2pForwardCmd,
		"close":   p2pCloseCmd,
		"ls":      p2pLsCmd,
	},
}

var p2pForwardCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Forward connections to or from libp2p services",
		ShortDescription: `
Forward connections to <listen-address> to <target-address>. Protocol specifies
the libp2p protocol to use.

To create libp2p service listener, specify '/ipfs' as <listen-address>

Examples:
  ipfs p2p forward myproto /ipfs /ip4/127.0.0.1/tcp/1234
    - Forward connections to 'myproto' libp2p service to 127.0.0.1:1234

  ipfs p2p forward myproto /ip4/127.0.0.1/tcp/4567 /ipfs/QmPeer
    - Forward connections to 127.0.0.1:4567 to 'myproto' service on /ipfs/QmPeer

`,
	},
	Arguments: []cmdkit.Argument{
		cmdkit.StringArg("protocol", true, false, "Protocol identifier."),
		cmdkit.StringArg("listen-address", true, false, "Listening endpoint"),
		cmdkit.StringArg("target-address", true, false, "Target endpoint."),
	},
	Run: func(req cmds.Request, res cmds.Response) {
		n, err := p2pGetNode(req)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		//TODO: Do we really want/need implicit prefix?
		proto := "/p2p/" + req.Arguments()[0]
		listen := req.Arguments()[1]
		target := req.Arguments()[2]

		if strings.HasPrefix(listen, "/ipfs") {
			if listen != "/ipfs" {
				res.SetError(errors.New("only '/ipfs' is allowed as libp2p listen address"), cmdkit.ErrNormal)
				return
			}

			if err := forwardRemote(n.Context(), n.P2P, proto, target); err != nil {
				res.SetError(err, cmdkit.ErrNormal)
				return
			}
		} else {
			if err := forwardLocal(n.Context(), n.P2P, n.Peerstore, proto, listen, target); err != nil {
				res.SetError(err, cmdkit.ErrNormal)
				return
			}
		}
		res.SetOutput(nil)
	},
}

// forwardRemote forwards libp2p service connections to a manet address
func forwardRemote(ctx context.Context, p *p2p.P2P, proto string, target string) error {
	if strings.HasPrefix(target, "/ipfs") {
		return errors.New("cannot forward libp2p service connections to another libp2p service")
	}

	addr, err := ma.NewMultiaddr(target)
	if err != nil {
		return err
	}

	// TODO: return some info
	_, err = p.ForwardRemote(ctx, proto, addr)
	return err
}

// forwardLocal forwards local connections to a libp2p service
func forwardLocal(ctx context.Context, p *p2p.P2P, ps pstore.Peerstore, proto string, listen string, target string) error {
	bindAddr, err := ma.NewMultiaddr(listen)
	if err != nil {
		return err
	}

	addr, peer, err := ParsePeerParam(target)
	if err != nil {
		return err
	}

	if addr != nil {
		ps.AddAddr(peer, addr, pstore.TempAddrTTL)
	}

	// TODO: return some info
	_, err = p.ForwardLocal(ctx, peer, proto, bindAddr)
	return err
}

var p2pLsCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "List active p2p listeners.",
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption("headers", "v", "Print table headers (Protocol, Listen, Target)."),
	},
	Run: func(req cmds.Request, res cmds.Response) {
		n, err := p2pGetNode(req)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		output := &P2PLsOutput{}

		for _, listener := range n.P2P.Listeners.Listeners {
			output.Listeners = append(output.Listeners, P2PListenerInfoOutput{
				Protocol:      listener.Protocol(),
				ListenAddress: listener.ListenAddress(),
				TargetAddress: listener.TargetAddress(),
			})
		}

		res.SetOutput(output)
	},
	Type: P2PLsOutput{},
	Marshalers: cmds.MarshalerMap{
		cmds.Text: func(res cmds.Response) (io.Reader, error) {
			v, err := unwrapOutput(res.Output())
			if err != nil {
				return nil, err
			}

			headers, _, _ := res.Request().Option("headers").Bool()
			list := v.(*P2PLsOutput)
			buf := new(bytes.Buffer)
			w := tabwriter.NewWriter(buf, 1, 2, 1, ' ', 0)
			for _, listener := range list.Listeners {
				if headers {
					fmt.Fprintln(w, "Protocol\tListen Address\tTarget Address")
				}

				fmt.Fprintf(w, "%s\t%s\t%s\n", listener.Protocol, listener.ListenAddress, listener.TargetAddress)
			}
			w.Flush()

			return buf, nil
		},
	},
}

var p2pCloseCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Stop listening for new connections to forward.",
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption("all", "a", "Close all listeners."),
		cmdkit.StringOption("protocol", "p", "Match protocol name"),
		cmdkit.StringOption("listen-address", "l", "Match listen address"),
		cmdkit.StringOption("target-address", "t", "Match target address"),
	},
	Run: func(req cmds.Request, res cmds.Response) {
		res.SetOutput(nil)

		n, err := p2pGetNode(req)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		closeAll, _, _ := req.Option("all").Bool()
		proto, p, _ := req.Option("protocol").String()
		listen, l, _ := req.Option("listen-address").String()
		target, t, _ := req.Option("target-address").String()

		if !(closeAll || p || l || t) {
			res.SetError(errors.New("no connection matching options given"), cmdkit.ErrNormal)
			return
		}

		if closeAll && (p || l || t) {
			res.SetError(errors.New("can't combine --all with other matching options"), cmdkit.ErrNormal)
			return
		}

		match := func(listener p2p.Listener) bool {
			out := true
			if p || !strings.HasPrefix(proto, "/p2p/") {
				proto = "/p2p/" + proto
			}

			if p {
				out = out && (proto == listener.Protocol())
			}
			if l {
				out = out && (listen == listener.ListenAddress())
			}
			if t {
				out = out && (target == listener.TargetAddress())
			}

			out = out || closeAll
			return out
		}

		var closed int
		for _, listener := range n.P2P.Listeners.Listeners {
			if !match(listener) {
				continue
			}
			listener.Close()
			closed++
		}
		res.SetOutput(closed)
	},
	Type: int(0),
	Marshalers: cmds.MarshalerMap{
		cmds.Text: func(res cmds.Response) (io.Reader, error) {
			v, err := unwrapOutput(res.Output())
			if err != nil {
				return nil, err
			}

			closed := v.(int)
			buf := new(bytes.Buffer)
			fmt.Fprintf(buf, "Closed %d stream(s)\n", closed)

			return buf, nil
		},
	},
}

///////
// Listener
//

// p2pStreamCmd is the 'ipfs p2p stream' command
var p2pStreamCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline:          "P2P stream management.",
		ShortDescription: "Create and manage p2p streams",
	},

	Subcommands: map[string]*cmds.Command{
		"ls":    p2pStreamLsCmd,
		"close": p2pStreamCloseCmd,
	},
}

var p2pStreamLsCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "List active p2p streams.",
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption("headers", "v", "Print table headers (HagndlerID, Protocol, Local, Remote)."),
	},
	Run: func(req cmds.Request, res cmds.Response) {
		n, err := p2pGetNode(req)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		output := &P2PStreamsOutput{}

		for id, s := range n.P2P.Streams.Streams {
			output.Streams = append(output.Streams, P2PStreamInfoOutput{
				HandlerID: strconv.FormatUint(id, 10),

				Protocol: s.Protocol,

				OriginAddress: s.OriginAddr.String(),
				TargetAddress: s.TargetAddr.String(),
			})
		}

		res.SetOutput(output)
	},
	Type: P2PStreamsOutput{},
	Marshalers: cmds.MarshalerMap{
		cmds.Text: func(res cmds.Response) (io.Reader, error) {
			v, err := unwrapOutput(res.Output())
			if err != nil {
				return nil, err
			}

			headers, _, _ := res.Request().Option("headers").Bool()
			list := v.(*P2PStreamsOutput)
			buf := new(bytes.Buffer)
			w := tabwriter.NewWriter(buf, 1, 2, 1, ' ', 0)
			for _, stream := range list.Streams {
				if headers {
					fmt.Fprintln(w, "Id\tProtocol\tOrigin\tTarget")
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", stream.HandlerID, stream.Protocol, stream.OriginAddress, stream.TargetAddress)
			}
			w.Flush()

			return buf, nil
		},
	},
}

var p2pStreamCloseCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Close active p2p stream.",
	},
	Arguments: []cmdkit.Argument{
		cmdkit.StringArg("id", false, false, "Stream identifier"),
	},
	Options: []cmdkit.Option{
		cmdkit.BoolOption("all", "a", "Close all streams."),
	},
	Run: func(req cmds.Request, res cmds.Response) {
		res.SetOutput(nil)

		n, err := p2pGetNode(req)
		if err != nil {
			res.SetError(err, cmdkit.ErrNormal)
			return
		}

		closeAll, _, _ := req.Option("all").Bool()
		var handlerID uint64

		if !closeAll {
			if len(req.Arguments()) == 0 {
				res.SetError(errors.New("no id specified"), cmdkit.ErrNormal)
				return
			}

			handlerID, err = strconv.ParseUint(req.Arguments()[0], 10, 64)
			if err != nil {
				res.SetError(err, cmdkit.ErrNormal)
				return
			}
		}

		for id, stream := range n.P2P.Streams.Streams {
			if !closeAll && handlerID != id {
				continue
			}
			stream.Reset()
			if !closeAll {
				break
			}
		}
	},
}

func p2pGetNode(req cmds.Request) (*core.IpfsNode, error) {
	n, err := req.InvocContext().GetNode()
	if err != nil {
		return nil, err
	}

	config, err := n.Repo.Config()
	if err != nil {
		return nil, err
	}

	if !config.Experimental.Libp2pStreamMounting {
		return nil, errors.New("libp2p stream mounting not enabled")
	}

	if !n.OnlineMode() {
		return nil, errNotOnline
	}

	return n, nil
}
