package monkey

import (
	"context"
	"fmt"
	"regexp"

	"github.com/ipfs/go-log/v2"
	seerIface "github.com/taubyte/go-interfaces/services/seer"
	ci "github.com/taubyte/go-simple-container/gc"
	tnsClient "github.com/taubyte/odo/clients/p2p/tns"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	dreamlandCommon "github.com/taubyte/dreamland/core/common"
	domainSpecs "github.com/taubyte/go-specs/domain"
	patrickSpecs "github.com/taubyte/go-specs/patrick"
	seerClient "github.com/taubyte/odo/clients/p2p/seer"
	odoConfig "github.com/taubyte/odo/config"

	"github.com/taubyte/odo/protocols/common"
	protocolCommon "github.com/taubyte/odo/protocols/common"
	streams "github.com/taubyte/p2p/streams/service"
)

var logger = log.Logger("monkey.service")

func (srv *Service) subscribe() error {
	return srv.node.PubSubSubscribe(
		patrickSpecs.PubSubIdent,
		func(msg *pubsub.Message) {
			go srv.pubsubMsgHandler(msg)
		},
		func(err error) {
			// re-establish if fails
			if err.Error() != "context canceled" {
				logger.Errorf("Subscription had an error: %w", err)
				if err := srv.subscribe(); err != nil {
					logger.Errorf("resubscribe failed with: %w", err)
				}
			}
		},
	)
}

func New(ctx context.Context, config *odoConfig.Protocol) (*Service, error) {
	if config == nil {
		config = &odoConfig.Protocol{}
	}

	err := config.Build(odoConfig.ConfigBuilder{
		DefaultP2PListenPort: protocolCommon.MonkeyDefaultP2PListenPort,
		DevP2PListenFormat:   dreamlandCommon.DefaultP2PListenFormat,
	})
	if err != nil {
		return nil, err
	}

	srv := &Service{
		ctx: ctx,
		dev: config.DevMode,
	}

	if !config.DevMode {
		domainSpecs.SpecialDomain = regexp.MustCompile(config.GeneratedDomain)
	}

	err = ci.Start(ctx, ci.Interval(ci.DefaultInterval), ci.MaxAge(ci.DefaultMaxAge))
	if err != nil {
		return nil, err
	}

	if config.Node == nil {
		srv.node, err = odoConfig.NewLiteNode(ctx, config, protocolCommon.Monkey)
		if err != nil {
			return nil, err
		}
	} else {
		srv.node = config.Node
		srv.dvPublicKey = config.DomainValidation.PublicKey
	}

	// For Odo
	if config.ClientNode != nil {
		srv.odoClientNode = config.ClientNode
	} else {
		srv.odoClientNode = srv.node
	}

	// should end if any of the two contexts ends
	err = srv.subscribe()
	if err != nil {
		return nil, err
	}

	srv.stream, err = streams.New(srv.node, protocolCommon.Monkey, protocolCommon.MonkeyProtocol)
	if err != nil {
		return nil, err
	}

	srv.setupStreamRoutes()

	sc, err := seerClient.New(ctx, srv.odoClientNode)
	if err != nil {
		return nil, fmt.Errorf("creating seer client failed with %s", err)
	}

	err = common.StartSeerBeacon(config, sc, seerIface.ServiceTypeMonkey)
	if err != nil {
		return nil, fmt.Errorf("starting seer beacon failed with %s", err)
	}

	srv.monkeys = make(map[string]*Monkey, 0)

	srv.patrickClient, err = NewPatrick(ctx, srv.odoClientNode)
	if err != nil {
		return nil, err
	}

	srv.tnsClient, err = tnsClient.New(ctx, srv.odoClientNode)
	if err != nil {
		return nil, err
	}

	return srv, nil
}

func (srv *Service) Close() error {
	logger.Info("Closing", protocolCommon.Monkey)
	defer logger.Info(protocolCommon.Monkey, "closed")

	srv.stream.Stop()

	srv.tnsClient.Close()
	srv.patrickClient.Close()

	return nil
}