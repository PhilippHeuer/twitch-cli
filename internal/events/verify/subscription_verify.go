// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package verify

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
	"mime"

	"github.com/fatih/color"
	"github.com/twitchdev/twitch-cli/internal/events/trigger"
	"github.com/twitchdev/twitch-cli/internal/events/types"
	"github.com/twitchdev/twitch-cli/internal/models"
	"github.com/twitchdev/twitch-cli/internal/util"
)

type VerifyParameters struct {
	Transport      string
	Event          string
	ForwardAddress string
	Secret         string
}

type VerifyResponse struct {
	IsStatusValid    bool
	IsChallengeValid bool
	Body             string
}

func VerifyWebhookSubscription(p VerifyParameters) (VerifyResponse, error) {
	r := VerifyResponse{}

	challenge := util.RandomGUID()

	event, err := types.GetByTriggerAndTransport(p.Event, p.Transport)
	if err != nil {
		return VerifyResponse{}, err
	}

	if p.Transport == models.TransportEventSub {
		newTrigger := event.GetEventSubAlias(p.Event)
		if newTrigger != "" {
			p.Event = newTrigger
		}
	}

	body, err := generateWebhookSubscriptionBody(p.Transport, event.GetTopic(p.Transport, p.Event), challenge, p.ForwardAddress)
	if err != nil {
		return VerifyResponse{}, err
	}

	r.Body = string(body.JSON)

	if p.ForwardAddress != "" {
		requestMethod := http.MethodPost
		u, err := url.Parse(p.ForwardAddress)
		if err != nil {
			return VerifyResponse{}, err
		}

		resp, err := trigger.ForwardEvent(trigger.ForwardParamters{
			ID:             body.ID,
			Event:          event.GetTopic(p.Transport, p.Event),
			JSON:           body.JSON,
			Transport:      p.Transport,
			Secret:         p.Secret,
			Method:         requestMethod,
			ForwardAddress: u.String(),
			Type:           trigger.EventSubMessageTypeVerification,
		})
		if err != nil {
			return VerifyResponse{}, err
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return VerifyResponse{}, err
		}

		respChallenge := string(body)
		if respChallenge == challenge {
			color.New().Add(color.FgGreen).Println(fmt.Sprintf(`✔ Valid response. Received challenge %s in body`, challenge))
			r.IsChallengeValid = true
		} else {
			color.New().Add(color.FgRed).Println(fmt.Sprintf(`✗ Invalid response. Received %s as body, expected %s`, respChallenge, challenge))
			r.IsChallengeValid = false
		}

		mediatype, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		charset := string(params["charset"])

		if err != nil {
			return VerifyResponse{}, err
		}

		if mediatype == "text/plain" {
			if charset != "" {
				color.New().Add(color.FgGreen).Println(fmt.Sprintf(`✔ Valid content-type header. Received type %v with charset %v`, mediatype, params["charset"]))
			} else {
				color.New().Add(color.FgGreen).Println(fmt.Sprintf(`✔ Valid content-type header. Received type %v`, mediatype))
			}
		} else {
			if charset != "" {
				color.New().Add(color.FgRed).Println(fmt.Sprintf(`✗ Invalid content-type header. Received type %v with charset %v`, mediatype, params["charset"]))
			} else {
				color.New().Add(color.FgRed).Println(fmt.Sprintf(`✗ Invalid content-type header. Received type %v`, mediatype))
			}
		}

		if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			color.New().Add(color.FgGreen).Println(fmt.Sprintf(`✔ Valid status code. Received status %v`, resp.StatusCode))
			r.IsStatusValid = true
		} else {
			color.New().Add(color.FgRed).Println(fmt.Sprintf(`✗ Invalid status code. Received %v, expected a 2XX status`, resp.StatusCode))
			r.IsStatusValid = false
		}
	}

	return r, nil
}

func generateWebhookSubscriptionBody(transport string, event string, challenge string, callback string) (trigger.TriggerResponse, error) {
	var res []byte
	var err error
	id := util.RandomGUID()
	ts := util.GetTimestamp().Format(time.RFC3339Nano)
	switch transport {
	case models.TransportEventSub:
		body := models.EventsubSubscriptionVerification{
			Challenge: challenge,
			Subscription: models.EventsubSubscription{
				ID:      id,
				Status:  "webhook_callback_verification_pending",
				Type:    event,
				Version: "1",
				Condition: models.EventsubCondition{
					BroadcasterUserID: util.RandomUserID(),
				},
				Transport: models.EventsubTransport{
					Method:   "webhook",
					Callback: callback,
				},
				CreatedAt: ts,
			},
		}
		res, err = json.Marshal(body)
		if err != nil {
			return trigger.TriggerResponse{}, err
		}
	default:
		res = []byte("")
	}
	return trigger.TriggerResponse{
		ID:        id,
		JSON:      res,
		Timestamp: ts,
	}, nil
}
