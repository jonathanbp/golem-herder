package metering

import (
	"fmt"
	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/boltdb/bolt"
)

var db *bolt.DB

func init() {
	meterdb, err := bolt.Open("meter.db", 0600, nil)
	if err != nil {
		panic(err)
	}
	db = meterdb
}

// NewMeter returns a new Meter for the given token
// Database is structured as follows:
// Each token has a Bucket with the token id as name.
// Each Bucket has the following properties:
// * MillisecondsUsed
// * MillisecondsRemaining
// * TODO figure out more stats to track
func NewMeter(id string, remainingMs int) (*Meter, error) {
	err := db.Update(func(tx *bolt.Tx) error {
		if b, err := tx.CreateBucket([]byte(id)); err == nil {
			b.Put([]byte("MillisecondsRemaining"), []byte(strconv.Itoa(remainingMs)))
			b.Put([]byte("MillisecondsUsed"), []byte("0"))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Meter{ID: id, db: db}, nil
}

// Meter will keep track of resources for a given minion or daemon
type Meter struct {
	ID string
	db *bolt.DB
}

// Status contains information about the current status of the resources of a token
type Status struct {
	MillisecondsRemaining int
}

func (m *Meter) MillisecondsRemaining() (int, error) {
	var msr int
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(m.ID))
		if b != nil {
			res := b.Get([]byte("MillisecondsRemaining"))
			if res != nil {
				ms, err := strconv.Atoi(string(res))
				if err != nil {
					return err
				}
				msr = ms
			}
		}
		return fmt.Errorf("Could not get time remaining")
	})
	if err != nil {
		return 0, err
	}
	return msr, nil
}

func (m *Meter) RecordMilliseconds(ms int) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(m.ID))
		if b != nil {
			var remaining, used int
			// Get remaining
			res := b.Get([]byte("MillisecondsRemaining"))
			if res != nil {
				ms, err := strconv.Atoi(string(res))
				if err != nil {
					return err
				}
				remaining = ms
			}
			// Do not record if >= 0
			if remaining <= 0 {
				return fmt.Errorf("Could not record time - no time left")
			}

			// Get used
			res = b.Get([]byte("MillisecondsUsed"))
			if res != nil {
				ms, err := strconv.Atoi(string(res))
				if err != nil {
					return err
				}
				used = ms
			}
			// Update
			err := b.Put([]byte("MillisecondsRemaining"), []byte(strconv.Itoa(remaining-ms)))
			if err != nil {
				return err
			}
			err = b.Put([]byte("MillisecondsUsed"), []byte(strconv.Itoa(used+ms)))
			if err != nil {
				return err
			}
			log.WithField("remaining", remaining-ms).Info("Recorded some time")
			return nil
		}
		return fmt.Errorf("Could not update for given id")
	})
}

func (m *Meter) Inspect() (*Status, error) {
	msr, err := m.MillisecondsRemaining()
	if err != nil {
		return nil, err
	}
	return &Status{MillisecondsRemaining: msr}, nil
}
