package main

import (
	"bytes"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"log"
	"net"
	"strings"
	"sync"
)

type Peer struct {
	Queued
	sync.RWMutex
	SpeedUP           float64 `redis:"speed_up" json:"speed_up"`
	SpeedDN           float64 `redis:"speed_dn" json:"speed_dn"`
	SpeedUPMax        float64 `redis:"speed_up" json:"speed_up_max"`
	SpeedDNMax        float64 `redis:"speed_dn" json:"speed_dn_max"`
	Uploaded          uint64  `redis:"uploaded" json:"uploaded"`
	Downloaded        uint64  `redis:"downloaded" json:"downloaded"`
	Corrupt           uint64  `redis:"corrupt" json:"corrupt"`
	IP                string  `redis:"ip" json:"ip"`
	Port              uint64  `redis:"port" json:"port"`
	Left              uint64  `redis:"left" json:"left"`
	Announces         uint64  `redis:"announces" json:"announces"`
	TotalTime         uint32  `redis:"total_time" json:"total_time"`
	AnnounceLast      int32   `redis:"last_announce" json:"last_announce"`
	AnnounceFirst     int32   `redis:"first_announce" json:"first_announce"`
	New               bool    `redis:"new" json:"-"`
	PeerID            string  `redis:"peer_id" json:"peer_id"`
	Active            bool    `redis:"active"  json:"active"`
	Username          string  `redis:"username"  json:"username"`
	UserID            uint64  `redis:"user_id"  json:"user_id"`
	TorrentID         uint64  `redis:"torrent_id" json:"torrent_id"`
	KeyPeer           string  `redis:"-" json:"-"`
	KeyUserActive     string  `redis:"-" json:"-"`
	KeyUserIncomplete string  `redis:"-" json:"-"`
	KeyUserComplete   string  `redis:"-" json:"-"`
	KeyUserHNR        string  `redis:"-" json:"-"`
}

// Update the stored values with the data from an announce
func (peer *Peer) Update(announce *AnnounceRequest) (ul uint64, dl uint64) {
	peer.Lock()
	defer peer.Unlock()
	cur_time := unixtime()
	peer.PeerID = announce.PeerID
	peer.Announces++

	ul = announce.Uploaded - peer.Uploaded
	dl = announce.Downloaded - peer.Downloaded
	if ul < 0 {
		ul = 0
	}
	if dl < 0 {
		dl = 0
	}
	// Change to int or byte?
	peer.Uploaded = announce.Uploaded
	peer.Downloaded = announce.Downloaded
	peer.IP = announce.IPv4.String()
	peer.Port = announce.Port
	peer.Corrupt = announce.Corrupt
	peer.Left = announce.Left
	peer.SpeedUP = estSpeed(peer.AnnounceLast, cur_time, ul)
	peer.SpeedDN = estSpeed(peer.AnnounceLast, cur_time, dl)
	if peer.SpeedUP > peer.SpeedUPMax {
		peer.SpeedUPMax = peer.SpeedUP
	}
	if peer.SpeedDN > peer.SpeedDNMax {
		peer.SpeedDNMax = peer.SpeedDN
	}

	// Must be active to have a real time delta
	if peer.Active && peer.AnnounceLast > 0 {
		time_diff := uint64(unixtime() - peer.AnnounceLast)
		// Ignore long periods of inactivity
		if time_diff < (uint64(config.AnnInterval) * 4) {
			peer.TotalTime += uint32(time_diff)
		}
	}
	return ul, dl
}

func (peer *Peer) SetUserID(user_id uint64, username string) {
	peer.Lock()
	defer peer.Unlock()
	peer.UserID = user_id
	peer.KeyUserActive = fmt.Sprintf("t:u:%d:active", user_id)
	peer.KeyUserIncomplete = fmt.Sprintf("t:u:%d:incomplete", user_id)
	peer.KeyUserComplete = fmt.Sprintf("t:u:%d:complete", user_id)
	peer.KeyUserHNR = fmt.Sprintf("t:u:%d:hnr", user_id)
	peer.Username = username
}

func (peer *Peer) Sync(r redis.Conn) {
	r.Send(
		"HMSET", peer.KeyPeer,
		"ip", peer.IP,
		"port", peer.Port,
		"left", peer.Left,
		"first_announce", peer.AnnounceFirst,
		"last_announce", peer.AnnounceLast,
		"total_time", peer.TotalTime,
		"speed_up", peer.SpeedUP,
		"speed_dn", peer.SpeedDN,
		"speed_up_max", peer.SpeedUPMax,
		"speed_dn_max", peer.SpeedDNMax,
		"active", peer.Active,
		"uploaded", peer.Uploaded,
		"downloaded", peer.Downloaded,
		"corrupt", peer.Corrupt,
		"username", peer.Username,
		"user_id", peer.UserID, // Shouldn't need to be here
		"peer_id", peer.PeerID, // Shouldn't need to be here
		"torrent_id", peer.TorrentID, // Shouldn't need to be here
	)
}

func (peer *Peer) IsHNR() bool {
	return peer.Left > 0 && peer.AnnounceFirst-unixtime() > config.HNRThreshold
}

func (peer *Peer) IsSeeder() bool {
	return peer.Left == 0
}

func (peer *Peer) AddHNR(r redis.Conn, torrent_id uint64) {
	r.Send("SADD", fmt.Sprintf("t:u:%d:hnr", peer.UserID), torrent_id)
	Debug("Added HnR:", torrent_id, peer.UserID)
}

// Generate a compact peer field array containing the byte representations
// of a peers IP+Port appended to each other
func makeCompactPeers(peers []*Peer, skip_id string) []byte {
	var out_buf bytes.Buffer
	for _, peer := range peers {
		if peer.Port <= 0 {
			// FIXME Why does empty peer exist with 0 port??
			continue
		}
		if peer.PeerID == skip_id {
			continue
		}

		out_buf.Write(net.ParseIP(peer.IP).To4())
		out_buf.Write([]byte{byte(peer.Port >> 8), byte(peer.Port & 0xff)})
	}
	return out_buf.Bytes()
}

// Generate a new instance of a peer from the redis reply if data is contained
// within, otherwise just return a default value peer
func makePeer(redis_reply interface{}, torrent_id uint64, peer_id string) (*Peer, error) {
	peer := &Peer{
		PeerID:        peer_id,
		Active:        false,
		Announces:     0,
		SpeedUP:       0,
		SpeedDN:       0,
		SpeedUPMax:    0,
		SpeedDNMax:    0,
		Uploaded:      0,
		Downloaded:    0,
		Left:          0,
		Corrupt:       0,
		Username:      "",
		IP:            "127.0.0.1",
		Port:          0,
		AnnounceFirst: unixtime(),
		AnnounceLast:  unixtime(),
		TotalTime:     0,
		UserID:        0,
		TorrentID:     torrent_id,
		KeyPeer:       fmt.Sprintf("t:t:%d:%s", torrent_id, peer_id),
	}

	values, err := redis.Values(redis_reply, nil)
	if err != nil {
		log.Println("Failed to parse peer reply: ", err)
		return peer, err_parse_reply
	}
	if values != nil {
		err := redis.ScanStruct(values, peer)
		if err != nil {
			log.Println("Failed to fetch peer: ", err)
			return peer, err_cast_reply
		} else {
			peer.PeerID = peer_id
		}
	}
	return peer, nil
}

// Checked if the clients peer_id prefix matches the client prefixes
// stored in the white lists
func IsValidClient(r redis.Conn, peer_id string) bool {

	for _, client_prefix := range whitelist {
		if strings.HasPrefix(peer_id, client_prefix) {
			return true
		}
	}

	log.Println("Got non-whitelisted client:", peer_id)
	return false
}
