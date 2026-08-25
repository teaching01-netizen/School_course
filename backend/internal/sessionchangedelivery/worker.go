package sessionchangedelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/emailnotifier"
	"warwick-institute/internal/smartsms"
)

type payload struct {
	Student       string   `json:"student"`
	Action        string   `json:"action"`
	SMSTemplate   string   `json:"sms_template"`
	SMSMessage    string   `json:"sms_message"`
	SMSMobiles    []string `json:"sms_mobiles"`
	SMSCampaignNo string   `json:"campaign_no"`
	SMSRefNo      string   `json:"ref_no"`
	EmailSubject  string   `json:"email_subject"`
	EmailBody     string   `json:"email_body"`
}

const notificationWorkerConcurrency = 4

type Worker struct {
	q     *sqldb.Queries
	sms   smartsms.SMSProvider
	email *emailnotifier.Service
	log   *slog.Logger
}

func New(q *sqldb.Queries, sms smartsms.SMSProvider, email *emailnotifier.Service, log *slog.Logger) *Worker {
	return &Worker{q: q, sms: sms, email: email, log: log}
}

func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for range notificationWorkerConcurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := w.RunOnce(ctx); err != nil && w.log != nil {
						w.log.Error("notification delivery failed", "error", err)
					}
				}
			}
		}()
	}
	wg.Wait()
}

func (w *Worker) RunOnce(ctx context.Context) error {
	item, err := w.q.NotificationOutboxClaimNext(ctx)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return fmt.Errorf("claim notification: %w", err)
	}
	message, err := decodePayload(item.Payload)
	if err == nil {
		switch item.Channel {
		case "sms":
			err = w.sendSMS(ctx, item.Recipient, message)
		case "email":
			err = w.sendEmail(ctx, item.Recipient, message)
		default:
			err = fmt.Errorf("unsupported notification channel %q", item.Channel)
		}
	}
	if err != nil {
		return w.q.NotificationOutboxFail(ctx, item.ID, item.AttemptCount, err.Error())
	}
	return w.q.NotificationOutboxDeliver(ctx, item.ID, "")
}

func decodePayload(raw []byte) (payload, error) {
	var item payload
	if err := json.Unmarshal(raw, &item); err != nil {
		return payload{}, fmt.Errorf("decode notification payload: %w", err)
	}
	return item, nil
}

func (w *Worker) sendSMS(ctx context.Context, recipient string, message payload) error {
	if w.sms == nil {
		return fmt.Errorf("SMS notification is not configured")
	}
	mobiles := message.SMSMobiles
	if len(mobiles) == 0 && strings.TrimSpace(recipient) != "" {
		mobiles = []string{recipient}
	}
	if len(mobiles) == 0 {
		return fmt.Errorf("SMS notification has no recipients")
	}
	content := message.SMSMessage
	if strings.TrimSpace(content) == "" {
		if strings.TrimSpace(message.SMSTemplate) == "" {
			return fmt.Errorf("SMS notification is not configured")
		}
		content = render(message.SMSTemplate, message)
	}
	campaignNo := message.SMSCampaignNo
	if strings.TrimSpace(campaignNo) == "" {
		campaignNo = "schedule-impact"
	}
	refNo := message.SMSRefNo
	if strings.TrimSpace(refNo) == "" {
		refNo = "schedule-impact"
	}
	result, err := w.sms.SendSMS(ctx, smartsms.SendRequest{
		CampaignNo: campaignNo, Campaign: campaignNo, Message: content,
		Mobiles: mobiles, RefNo: refNo,
	})
	if err != nil {
		return err
	}
	if result == nil || !result.Success {
		return fmt.Errorf("SMS provider rejected delivery")
	}
	return nil
}

func (w *Worker) sendEmail(ctx context.Context, recipient string, message payload) error {
	if w.email == nil || strings.TrimSpace(message.EmailSubject) == "" || strings.TrimSpace(message.EmailBody) == "" {
		return fmt.Errorf("email notification is not configured")
	}
	result := w.email.SendEmails(ctx, emailnotifier.SendInput{
		Template:   emailnotifier.Template{Subject: message.EmailSubject, Body: message.EmailBody},
		Recipients: []string{recipient}, Values: values(message),
	})
	if result.SentCount != 1 {
		if len(result.Outcomes) > 0 && result.Outcomes[0].Error != "" {
			return fmt.Errorf("email delivery failed: %s", result.Outcomes[0].Error)
		}
		return fmt.Errorf("email provider did not accept delivery")
	}
	return nil
}

func render(template string, message payload) string {
	result := template
	for placeholder, value := range values(message) {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

func values(message payload) map[string]string {
	return map[string]string{
		"{{student_name}}": message.Student,
		"{{action}}":       message.Action,
	}
}
