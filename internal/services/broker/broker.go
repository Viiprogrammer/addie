package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/MindHunter86/addie/internal/utils"
	"github.com/centrifugal/centrifuge-go"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"

	"github.com/golang-jwt/jwt"
)

type Broker struct {
	client *centrifuge.Client

	log *zerolog.Logger

	done <-chan struct{}
}

type ChatMessage struct {
	Input string `json:"input"`
}

func connToken(user string, exp int64, token string) string {
	// NOTE that JWT must be generated on backend side of your application!
	// Here we are generating it on client side only for example simplicity.
	claims := jwt.MapClaims{"sub": user}
	if exp > 0 {
		claims["exp"] = exp
	}
	t, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(token))
	if err != nil {
		panic(err)
	}
	return t
}

func NewBroker(c context.Context) (_ *Broker, e error) {
	broker := &Broker{}

	cli := c.Value(utils.ContextKeyCliContext).(*cli.Context)
	log := c.Value(utils.ContextKeyLogger).(*zerolog.Logger)
	broker.log = log

	log.Info().Msg("OK")

	hostname, e := os.Hostname()
	if e != nil {
		return
	}

	log.Print("OK")

	customDialContext := func(d *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
		return d.DialContext
	}

	client := centrifuge.NewJsonClient(cli.String("centrifugo-endpoint"), centrifuge.Config{
		// Token: connToken("49", 0, cli.String("centrifugo-token")),

		Name:    fmt.Sprintf("%s on %s", cli.App.Name, hostname),
		Version: cli.App.Version,

		NetDialContext: customDialContext(&net.Dialer{
			Timeout:   cli.Duration("centrifugo-conn-timeout"),
			KeepAlive: cli.Duration("centrifugo-keepalive"),
		}),

		// TLSConfig: &tls.Config{
		// 	InsecureSkipVerify: cli.Bool("centrifugo-insecure"), // skipcq: GSC-G402 false-positive
		// 	MinVersion:         tls.VersionTLS12,
		// 	MaxVersion:         tls.VersionTLS13,
		// },
	})
	broker.client = client

	log.Print("OK")

	client.OnConnecting(func(e centrifuge.ConnectingEvent) {
		log.Printf("Connecting - %d (%s)", e.Code, e.Reason)
	})
	client.OnConnected(func(e centrifuge.ConnectedEvent) {
		log.Printf("Connected with ID %s", e.ClientID)
	})
	client.OnDisconnected(func(e centrifuge.DisconnectedEvent) {
		log.Printf("Disconnected: %d (%s)", e.Code, e.Reason)
	})

	client.OnError(func(e centrifuge.ErrorEvent) {
		log.Printf("Error: %s", e.Error.Error())
	})

	client.OnMessage(func(e centrifuge.MessageEvent) {
		log.Printf("Message from server: %s", string(e.Data))
	})

	client.OnSubscribed(func(e centrifuge.ServerSubscribedEvent) {
		log.Printf("Subscribed to server-side channel %s: (was recovering: %v, recovered: %v)", e.Channel, e.WasRecovering, e.Recovered)
	})
	client.OnSubscribing(func(e centrifuge.ServerSubscribingEvent) {
		log.Printf("Subscribing to server-side channel %s", e.Channel)
	})
	client.OnUnsubscribed(func(e centrifuge.ServerUnsubscribedEvent) {
		log.Printf("Unsubscribed from server-side channel %s", e.Channel)
	})

	client.OnPublication(func(e centrifuge.ServerPublicationEvent) {
		log.Printf("Publication from server-side channel %s: %s (offset %d)", e.Channel, e.Data, e.Offset)
	})
	client.OnJoin(func(e centrifuge.ServerJoinEvent) {
		log.Printf("Join to server-side channel %s: %s (%s)", e.Channel, e.User, e.Client)
	})
	client.OnLeave(func(e centrifuge.ServerLeaveEvent) {
		log.Printf("Leave from server-side channel %s: %s (%s)", e.Channel, e.User, e.Client)
	})

	broker.done = c.Done()
	return broker, e
}

func (m *Broker) Bootstrap() (e error) {

	time.Sleep(5 * time.Second)

	defer m.client.Close()

	go func() {
		log := m.log

		if e = m.client.Connect(); e != nil {
			return
		}

		sub, err := m.client.NewSubscription("config:index", centrifuge.SubscriptionConfig{
			Recoverable: true,
			JoinLeave:   true,
		})
		if err != nil {
			e = err
			return
		}

		sub.OnSubscribing(func(e centrifuge.SubscribingEvent) {
			log.Printf("Subscribing on channel %s - %d (%s)", sub.Channel, e.Code, e.Reason)
		})
		sub.OnSubscribed(func(e centrifuge.SubscribedEvent) {
			log.Printf("Subscribed on channel %s, (was recovering: %v, recovered: %v)", sub.Channel, e.WasRecovering, e.Recovered)
		})
		sub.OnUnsubscribed(func(e centrifuge.UnsubscribedEvent) {
			log.Printf("Unsubscribed from channel %s - %d (%s)", sub.Channel, e.Code, e.Reason)
		})

		sub.OnError(func(e centrifuge.SubscriptionErrorEvent) {
			log.Printf("Subscription error %s: %s", sub.Channel, e.Error)
		})

		sub.OnPublication(func(e centrifuge.PublicationEvent) {
			var chatMessage *ChatMessage
			err := json.Unmarshal(e.Data, &chatMessage)
			if err != nil {
				return
			}
			log.Printf("Someone says via channel %s: %s (offset %d)", sub.Channel, chatMessage.Input, e.Offset)
		})
		sub.OnJoin(func(e centrifuge.JoinEvent) {
			log.Printf("Someone joined %s: user id %s, client id %s", sub.Channel, e.User, e.Client)
		})
		sub.OnLeave(func(e centrifuge.LeaveEvent) {
			log.Printf("Someone left %s: user id %s, client id %s", sub.Channel, e.User, e.Client)
		})

		err = sub.Subscribe()
		if err != nil {
			e = err
			return
		}
	}()

	<-m.done
	return
}
