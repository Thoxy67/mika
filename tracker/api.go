package tracker

import (
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/leighmacdonald/mika/config"
	"github.com/leighmacdonald/mika/consts"
	"github.com/leighmacdonald/mika/geo"
	"github.com/leighmacdonald/mika/metrics"
	"github.com/leighmacdonald/mika/store"
	"github.com/leighmacdonald/mika/util"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"time"
)

// StatusResp is a generic response struct used when simple responses are all that
// is required.
type StatusResp struct {
	Err     string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// Error implements the error interface for our response
func (s StatusResp) Error() string {
	return s.Err
}

// AdminAPI is the interface for administering a live server over HTTP
type AdminAPI struct {
	t *Tracker
}

// PingRequest represents a JSON ping request
type PingRequest struct {
	Ping string `json:"ping"`
}

// PingResponse represents a JSON ping response
type PingResponse struct {
	Pong string `json:"pong"`
}

func (a *AdminAPI) whitelistAdd(c *gin.Context) {
	var wcl store.WhiteListClient
	if err := c.BindJSON(&wcl); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if wcl.ClientPrefix == "" || wcl.ClientName == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if err := a.t.torrents.WhiteListAdd(wcl); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
	}
	a.t.Lock()
	a.t.Whitelist[wcl.ClientPrefix] = wcl
	a.t.Unlock()
	c.JSON(http.StatusOK, nil)
}

func (a *AdminAPI) whitelistDelete(c *gin.Context) {
	prefix := c.Param("prefix")
	if prefix == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	a.t.RLock()
	defer a.t.RUnlock()
	wlc := a.t.Whitelist[prefix]
	if err := a.t.torrents.WhiteListDelete(wlc); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	newWL := make(map[string]store.WhiteListClient)
	wl, err := a.t.torrents.WhiteListGetAll()
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	for _, w := range wl {
		newWL[w.ClientPrefix] = w
	}
	a.t.Lock()
	a.t.Whitelist = newWL
	a.t.Unlock()
	c.JSON(http.StatusOK, nil)
}

func (a *AdminAPI) whitelistGet(c *gin.Context) {
	var wl []store.WhiteListClient
	a.t.RLock()
	defer a.t.RUnlock()
	for _, c := range a.t.Whitelist {
		wl = append(wl, c)
	}
	c.JSON(http.StatusOK, wl)
}

func (a *AdminAPI) ping(c *gin.Context) {
	var r PingRequest
	if err := c.BindJSON(&r); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	c.JSON(http.StatusOK, PingResponse{Pong: r.Ping})
}

func infoHashFromCtx(infoHash *store.InfoHash, c *gin.Context, hex bool) bool {
	ihStr := c.Param("info_hash")
	if ihStr == "" {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"message": "Invalid info hash",
		})
		return false
	}

	if hex {
		if err := store.InfoHashFromHex(infoHash, ihStr); err != nil {
			log.Warnf("failed to parse info hash hex value from request context: %s", err.Error())
			return false
		}
	} else {
		if err := store.InfoHashFromString(infoHash, ihStr); err != nil {
			log.Warnf("failed to parse info hash from request context: %s", err.Error())
			return false
		}
	}
	return true
}

// TorrentAddRequest represents a JSON request for adding a new torrent
type TorrentAddRequest struct {
	Name     string  `json:"name"`
	InfoHash string  `json:"info_hash"`
	MultiUp  float64 `json:"multi_up"`
	MultiDn  float64 `json:"multi_dn"`
}

func (a *AdminAPI) torrentAdd(c *gin.Context) {
	var req TorrentAddRequest
	if err := c.BindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, StatusResp{Err: "Malformed request"})
		return
	}
	var t store.Torrent
	var ih store.InfoHash
	if err := store.InfoHashFromHex(&ih, req.InfoHash); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, StatusResp{Err: err.Error()})
		return
	}
	t.InfoHash = ih
	if req.MultiUp < 0 {
		t.MultiUp = 0
	} else {
		t.MultiUp = req.MultiUp
	}
	if req.MultiDn < 0 {
		t.MultiDn = 0
	} else {
		t.MultiDn = req.MultiDn
	}
	if err := a.t.torrents.Add(t); err != nil {
		if errors.Is(err, consts.ErrDuplicate) {
			c.AbortWithStatusJSON(http.StatusConflict, StatusResp{
				Err: err.Error(),
			})
			return
		}
		c.AbortWithStatusJSON(http.StatusBadRequest, StatusResp{Err: err.Error()})
		return
	}
	c.JSON(http.StatusOK, StatusResp{Message: "Torrent added successfully"})
}

func (a *AdminAPI) torrentDelete(c *gin.Context) {
	var infoHash store.InfoHash
	if !infoHashFromCtx(&infoHash, c, true) {
		return
	}
	if err := a.t.torrents.Delete(infoHash, true); err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{})
		return
	}
	c.JSON(http.StatusOK, StatusResp{Message: "Deleted successfully"})
}

// TorrentUpdatePrams defines what parameters we accept for updating a torrent. This is only
// a subset of the fields as not all should be considered mutable
type TorrentUpdatePrams struct {
	IsDeleted bool   `json:"is_deleted"`
	IsEnabled bool   `json:"is_enabled"`
	Reason    string `json:"reason"`
}

// TODO ability to un-delete a torrent
func (a *AdminAPI) torrentUpdate(c *gin.Context) {
	var ih store.InfoHash
	if !infoHashFromCtx(&ih, c, true) {
		return
	}
	var t store.Torrent
	err := a.t.torrents.Get(&t, ih, true)
	if err == consts.ErrInvalidInfoHash {
		c.JSON(http.StatusNotFound, gin.H{})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{})
		return
	}
	var tup store.TorrentUpdate
	if err := c.BindJSON(&tup); err != nil {
		c.JSON(http.StatusBadRequest, StatusResp{Err: err.Error()})
		return
	}
	if len(tup.Keys) == 0 {
		c.JSON(http.StatusBadRequest, StatusResp{Err: "no update keys specified"})
		return
	}
	for _, k := range tup.Keys {
		switch k {
		case "is_deleted":
			t.IsDeleted = tup.IsDeleted
		case "is_enabled":
			t.IsEnabled = tup.IsEnabled
		case "reason":
			t.Reason = tup.Reason
		case "multi_up":
			t.MultiUp = tup.MultiUp
		case "multi_dn":
			t.MultiDn = tup.MultiDn
		}
	}
	if err := a.t.torrents.Update(t); err != nil {
		c.JSON(http.StatusBadRequest, StatusResp{Err: err.Error()})
	} else {
		c.JSON(http.StatusOK, StatusResp{Message: "Updated successfully"})
	}
}

func (a *AdminAPI) userUpdate(c *gin.Context) {
	var user store.User
	passkey := c.Param("passkey")
	if len(passkey) != 20 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err := a.t.users.GetByPasskey(&user, passkey); err != nil {
		if errors.Is(consts.ErrUnauthorized, err) {
			c.AbortWithStatus(http.StatusNotFound)
		} else {
			c.AbortWithStatus(http.StatusInternalServerError)
		}
		return
	}
	var update store.User
	if err := c.BindJSON(&update); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if err := a.t.users.Update(update, passkey); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.AbortWithStatus(http.StatusOK)
}

// UserDeleteRequest represents a JSON API requests to delete a user via passkey
type UserDeleteRequest struct {
	Passkey string `json:"passkey"`
}

func (a *AdminAPI) userDelete(c *gin.Context) {
	pk := c.Param("passkey")
	var user store.User
	if err := a.t.users.GetByPasskey(&user, pk); err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, StatusResp{Err: "User not found"})
		return
	}
	if err := a.t.users.Delete(user); err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, StatusResp{Err: "Failed to delete user"})
		return
	}
	c.JSON(http.StatusOK, StatusResp{Message: "Deleted user successfully"})
}

func (a *AdminAPI) userAdd(c *gin.Context) {
	var user store.User
	if err := c.BindJSON(&user); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, StatusResp{Err: "Malformed request"})
		return
	}
	if user.Passkey == "" {
		user.Passkey = util.NewPasskey()
	}
	if err := a.t.users.Add(user); err != nil {
		log.Error(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, StatusResp{Err: "Failed to add user"})
		return
	}
	c.AbortWithStatus(http.StatusOK)
}

// ConfigRequest holds new config values for the tracker
//
// Duration string format follows golang time.Duration string format i.e.:
// 		A duration string a sequence of decimal numbers, each
//		with optional fraction and a unit suffix, such as "300ms", "1.5h" or "2h45m".
//		Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
type ConfigRequest struct {
	// The keys that we actually want to update from our struct
	// Some default values could be valid values so we cannot rely on empty values alone
	// Keys not listed are NOT updated even if a value is set in the struct
	UpdateKeys                 []config.Key `json:"update_keys"`
	TrackerAnnounceInterval    int          `json:"tracker_announce_interval,omitempty"`
	TrackerAnnounceIntervalMin int          `json:"tracker_announce_interval_min,omitempty"`
	TrackerReaperInterval      int          `json:"tracker_reaper_interval,omitempty"`
	TrackerBatchUpdateInterval int          `json:"tracker_batch_update_interval,omitempty"`
	TrackerMaxPeers            int          `json:"tracker_max_peers,omitempty"`
	TrackerAutoRegister        bool         `json:"tracker_auto_register"`
	TrackerAllowNonRoutable    bool         `json:"tracker_allow_non_routable"`
	GeodbEnabled               bool         `json:"geodb_enabled"`
}

func (a *AdminAPI) configGet(c *gin.Context) {
	cfg := ConfigRequest{
		TrackerAnnounceInterval:    int(a.t.AnnInterval.Seconds()),
		TrackerAnnounceIntervalMin: int(a.t.AnnIntervalMin.Seconds()),
		TrackerReaperInterval:      int(a.t.ReaperInterval.Seconds()),
		TrackerBatchUpdateInterval: int(a.t.BatchInterval.Seconds()),
		TrackerMaxPeers:            a.t.MaxPeers,
		TrackerAutoRegister:        a.t.AutoRegister,
		TrackerAllowNonRoutable:    a.t.AllowNonRoutable,
		GeodbEnabled:               a.t.GeodbEnabled,
	}
	c.JSON(200, cfg)
}

func (a *AdminAPI) configUpdate(c *gin.Context) {
	var configValues ConfigRequest
	var err error
	internalErr := false
	if err = c.BindJSON(&configValues); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{})
		return
	}
	a.t.Lock()
	defer a.t.Unlock()

	for _, k := range configValues.UpdateKeys {
		switch k {
		case config.TrackerAnnounceInterval:
			d, err := time.ParseDuration(fmt.Sprintf("%ds", configValues.TrackerAnnounceInterval))
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, StatusResp{Err: "Announce interval invalid format"})
				return
			}
			a.t.AnnInterval = d
		case config.TrackerAnnounceIntervalMin:
			d, err := time.ParseDuration(fmt.Sprintf("%ds", configValues.TrackerAnnounceIntervalMin))
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, StatusResp{Err: "Announce interval min invalid format"})
				return
			}
			a.t.AnnIntervalMin = d
		case config.TrackerReaperInterval:
			d, err := time.ParseDuration(fmt.Sprintf("%ds", configValues.TrackerReaperInterval))
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, StatusResp{Err: "Reaper interval invalid"})
				return
			}
			a.t.ReaperInterval = d
		case config.TrackerBatchUpdateInterval:
			d, err := time.ParseDuration(fmt.Sprintf("%ds", configValues.TrackerBatchUpdateInterval))
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, StatusResp{Err: "Batch interval invalid"})
				return
			}
			a.t.BatchInterval = d
		case config.TrackerMaxPeers:
			a.t.MaxPeers = configValues.TrackerMaxPeers
		case config.TrackerAutoRegister:
			a.t.AutoRegister = configValues.TrackerAutoRegister
		case config.TrackerAllowNonRoutable:
			a.t.AllowNonRoutable = configValues.TrackerAllowNonRoutable
		case config.GeodbEnabled:
			if configValues.GeodbEnabled && !a.t.GeodbEnabled {
				size := int64(0)
				key := config.GetString(config.GeodbAPIKey)
				outPath := config.GetString(config.GeodbPath)
				if util.Exists(outPath) {
					f, err := os.Open(outPath)
					if err != nil {
						internalErr = true
						break
					}
					fi, err := f.Stat()
					if err != nil {
						internalErr = true
						break
					}
					size = fi.Size()
				}
				if size == 0 || !util.Exists(outPath) {
					err = geo.DownloadDB(outPath, key)
					if err != nil {
						internalErr = true
						break
					}
				}
				newDb, err := geo.New(outPath)
				if err != nil {
					internalErr = true
					break
				}
				a.t.Geodb = newDb
				a.t.GeodbEnabled = true
			} else if !configValues.GeodbEnabled && a.t.GeodbEnabled {
				a.t.Geodb = &geo.DummyProvider{}
				a.t.GeodbEnabled = false
			}
		}
	}
	if err != nil {
		code := http.StatusBadRequest
		if internalErr {
			code = http.StatusInternalServerError
		}
		c.JSON(code, StatusResp{Err: err.Error()})
	} else {
		c.JSON(http.StatusOK, StatusResp{Message: "Config values updated"})
	}
}

func (a *AdminAPI) metrics(c *gin.Context) {
	stats := metrics.Get()
	c.String(200, stats.String())
}

// NewAPIHandler configures a router to handle API requests
func NewAPIHandler(tkr *Tracker) *gin.Engine {
	r := newRouter()
	h := AdminAPI{t: tkr}

	r.GET("/metrics", h.metrics)

	r.POST("/ping", h.ping)
	r.PATCH("/config", h.configUpdate)
	r.GET("/config", h.configGet)

	r.DELETE("/torrent/:info_hash", h.torrentDelete)
	r.PATCH("/torrent/:info_hash", h.torrentUpdate)
	r.POST("/torrent", h.torrentAdd)

	r.POST("/user", h.userAdd)
	r.DELETE("/user/pk/:passkey", h.userDelete)
	r.PATCH("/user/pk/:passkey", h.userUpdate)

	r.POST("/whitelist", h.whitelistAdd)
	r.DELETE("/whitelist/:prefix", h.whitelistDelete)
	r.GET("/whitelist", h.whitelistGet)
	r.NoRoute(noRoute)
	return r
}
