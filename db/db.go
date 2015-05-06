package db

import (
	"github.com/garyburd/redigo/redis"
	"log"
	"time"
)

var (
	Pool *redis.Pool

	// Channels
	SyncPayloadC = make(chan Payload, 1000)
)

type DBEntity interface {
	Sync(r redis.Conn) bool
}

type Queued struct {
	InQueue bool `redis:"-" json:"-"`
}

// Defined a single payload to send to the backend data store (redis)
type Payload struct {
	Command string
	Args    []interface{}
}

func NewPayload(command string, args ...interface{}) Payload {
	if len(args) < 1 {
		panic("Not enough arguments to make payload")
	}
	return Payload{Command: command, Args: args}
}

//
type BulkPayload struct {
	Payloads []Payload
}

func (db *BulkPayload) AddPayload(payload ...Payload) {
	db.Payloads = append(db.Payloads, payload...)

}

func Setup(host string, pass string) {
	if Pool != nil {
		// Close the existing pool cleanly if it exists
		err := Pool.Close()
		if err != nil {
			log.Fatalln("Cannot close existing redis pool:", err.Error())
		}
	}
	pool := &redis.Pool{
		MaxIdle:     0,
		IdleTimeout: 600 * time.Second,
		Wait:        true,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", host)
			if err != nil {
				return nil, err
			}
			if pass != "" {
				if _, err := c.Do("AUTH", pass); err != nil {
					c.Close()
					return nil, err
				}
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			if err != nil {
				// TODO remove me, temp hack to allow supervisord to reload process
				// since we currently don't actually handle graceful reconnects yet.
				log.Fatalln("Bad redis voodoo! exiting!", err)
			}
			return err
		},
	}
	Pool = pool
}
