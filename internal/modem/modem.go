package modem

import (
	"context"
	"log"
	"time"

	"github.com/jpillora/backoff"
	"github.com/warthog618/goatsms/internal/db"
	"github.com/warthog618/modem/at"
	"github.com/warthog618/modem/gsm"
	"github.com/warthog618/modem/serial"
	"github.com/warthog618/modem/trace"
)

type GSMModem struct {
	comPort  string
	baudrate int
	deviceID string
	trace    *log.Logger
}

func New(comPort string, baudrate int, deviceID string) (modem *GSMModem) {
	return &GSMModem{comPort: comPort, baudrate: baudrate, deviceID: deviceID}
}

type SMSDispatcher interface {
	Req() <-chan db.SMS
	Rsp() chan<- db.SMS
}

func (m *GSMModem) Connect(ctx context.Context, ss SMSDispatcher) {
	go m.monitor(ctx, ss)
}

func (m *GSMModem) monitor(ctx context.Context, ss SMSDispatcher) {
	connect := time.NewTimer(0) // for immediate connection
	b := backoff.Backoff{       // !!! configurable Min and Max, and Factor??
		Min: time.Second,
		Max: 5 * time.Minute,
	}
	log.Println("modem created:", m.deviceID)
	for {
		select {
		case <-ctx.Done():
			if !connect.Stop() {
				<-connect.C
			}
			return
		case <-connect.C:
			s, err := serial.New(m.comPort, m.baudrate)
			if err != nil {
				connect.Reset(b.Duration())
				continue
			}
			var modem *gsm.GSM
			if m.trace != nil {
				tr := trace.New(s, m.trace)
				modem = gsm.New(tr)
			} else {
				modem = gsm.New(s)
			}
			ictx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = modem.Init(ictx)
			cancel()
			if err != nil {
				connect.Reset(b.Duration())
				continue
			}
			log.Println("modem connected:", m.deviceID)
			b.Reset()

			go m.sender(ctx, modem, ss.Req(), ss.Rsp())
			// !!! Add other status monitors, such as signal strength

			select {
			case <-ctx.Done():
				return
			case <-modem.Closed():
				log.Println("modem disconnected:", m.deviceID)
				connect.Reset(b.Duration())
			}
		}
	}
}

// Sender is responsible for taking SMSs from the req channel, sending them
// via the modem, and returning the updated SMS to the response channel.
func (m *GSMModem) sender(ctx context.Context, modem *gsm.GSM, req <-chan db.SMS, rsp chan<- db.SMS) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-modem.Closed():
			return
		case sms, ok := <-req:
			if !ok {
				return
			}
			log.Println("sending: ", sms.UUID, m.deviceID)
			tctx, cancel := context.WithTimeout(ctx, 15*time.Second) // !!! configurable
			_, err := modem.SendSMS(tctx, sms.Mobile, sms.Body)
			cancel()
			// a bit leary about handling SMS state here - would prefer to do that in sender.go
			// but then the response sent to the sender becomes more complex.
			switch err {
			case nil:
				sms.Status = db.SMSSent
				sms.Device = m.deviceID
			case at.ErrClosed:
				rsp <- sms
				return
			case context.Canceled:
				// !!! handle other errors that indicate a problem with the modem or network, NOT the SMS itself.
				// such as different CMS or CME errors.
			case context.DeadlineExceeded:
				// Assume modem is dead???
				// !!! How to signal that to everyone else??
				// Need to, or just wait to see what happens elsewhere???
			default:
				if sms.Retries >= db.SMSRetryLimit {
					sms.Status = db.SMSErrored
				} else {
					sms.Retries++
				}
			}
			rsp <- sms
		}
	}
}
