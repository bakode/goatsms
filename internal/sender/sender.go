package sender

import (
	"context"
	"time"

	store "github.com/warthog618/goatsms/internal/db"
)

// Sender represents a dispatcher responsible for pulling pending SMSs from
// the database and farming them out to the modems that physically send them.
type Sender struct {
	add      chan store.SMS
	req      chan store.SMS
	rsp      chan store.SMS
	pool     map[string]bool
	poolSize int
	poolLow  int
}

// New creates a new Sender.
func New(poolSize, poolLow int) *Sender {
	return &Sender{
		add:      make(chan store.SMS),
		req:      make(chan store.SMS),
		rsp:      make(chan store.SMS),
		pool:     make(map[string]bool),
		poolSize: poolSize,
		poolLow:  poolLow,
	}
}

// AddMessage adds an SMS to be sent.
func (s *Sender) AddMessage(sms store.SMS) {
	s.add <- sms
}

// Req returns the channel on which modems should receive messages to be sent.
func (s *Sender) Req() <-chan store.SMS {
	return s.req
}

// Rsp returns the channel on which modems should send processed messages.
func (s *Sender) Rsp() chan<- store.SMS {
	return s.rsp
}

// Run peforms the core functionality of the Sender.
// It pulls messages from the database and passes them out to modems, via the req channel.
// The modems return processed messages via the rsp channel.
// It adds messages to be sent, to both the database and the pool, via the add channel.
func (s *Sender) Run(ctx context.Context, db *store.DB, pollPeriod time.Duration) {
	t := time.NewTimer(pollPeriod)
	defer func() {
		if !t.Stop() {
			<-t.C
		}
	}()

	backlogged := s.fillPool(db)
	for {
		select {
		case <-ctx.Done():
			// perform a controlled shutdown
			close(s.req)
			s.drainReq()
			for len(s.pool) > 0 {
				sms := <-s.rsp
				db.UpdateMessageStatus(sms)
				delete(s.pool, sms.UUID)
			}
			return
		case sms := <-s.add:
			db.InsertMessage(sms)
			if len(s.pool) < s.poolSize && !backlogged {
				s.pool[sms.UUID] = true
				s.req <- sms
			}
		case sms := <-s.rsp:
			db.UpdateMessageStatus(sms)
			if sms.Status == store.SMSPending {
				s.req <- sms
			} else {
				delete(s.pool, sms.UUID)
				// refill the pool if we're backlogged and below the low threshold
				// or if we're about to go idle (to double check we really are idle).
				if len(s.pool) == 0 || (len(s.pool) < s.poolLow && backlogged) {
					backlogged = s.fillPool(db)
				}
			}
		case <-t.C:
			// periodically refill the pool in case SMSs have been injected into the DB behind our back.
			t.Reset(pollPeriod)
			backlogged = s.fillPool(db)
		}
	}
}

// fillPool fills the pending set (the pool) with messages from the db.
// Returns true if there are more messages pending than we can currently
// fit in the pool (i.e. backlogged).
func (s *Sender) fillPool(db *store.DB) (backlogged bool) {
	pendingMsgs, err := db.GetPendingMessages(s.poolSize)
	if err != nil {
		// !!! not sure what to do in this case - assume it is transient and
		return false
	}
	if len(pendingMsgs) >= s.poolSize {
		backlogged = true
	}
	for _, sms := range pendingMsgs {
		if !s.pool[sms.UUID] {
			s.pool[sms.UUID] = true
			s.req <- sms
			// the set from db is not necessarily a superset of pool,
			// so prevent the pending pool overflowing...
			if len(s.pool) >= s.poolSize {
				break
			}
		}
	}
	return backlogged
}

// drainReq removes pending requests from the req channel to expidite a controlled shutdown.
func (s *Sender) drainReq() {
	for {
		sms, ok := <-s.req
		if !ok {
			return
		}
		delete(s.pool, sms.UUID)
	}
}
