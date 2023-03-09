package api

import (
	"net/http"
	"time"

	"github.com/bloxapp/ssv/utils/format"
	"github.com/gin-contrib/cache"
	"github.com/gin-contrib/cache/persistence"
	"github.com/gin-gonic/gin"
	"github.com/stakestar/startracker/db"
	"github.com/ulule/limiter/v3"
	ginlimiter "github.com/ulule/limiter/v3/drivers/middleware/gin"
	"github.com/ulule/limiter/v3/drivers/store/memory"
	"go.uber.org/zap"
)

type Api struct {
	db     *db.BoltDB
	logger *zap.Logger
}

func New(logger *zap.Logger, db *db.BoltDB) *Api {
	return &Api{
		db:     db,
		logger: logger,
	}
}

func (api *Api) Start() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// create a rate limiter
	rate := limiter.Rate{
		Limit:  10,
		Period: time.Minute,
	}
	store := memory.NewStore()
	limit := limiter.New(store, rate)

	// use the rate limiter middleware
	router.Use(ginlimiter.NewMiddleware(limit))

	// add cache middleware
	cacheStore := persistence.NewInMemoryStore(time.Minute)

	router.GET("/api/nodes", cache.CachePage(cacheStore, time.Minute, api.GetNodes))
	router.GET("/api/nodes/pubkey/:pubkey", cache.CachePage(cacheStore, time.Minute, api.GetNodeByPubKey))
	router.GET("/api/nodes/operatorid/:operatorid", cache.CachePage(cacheStore, time.Minute, api.GetNodeByOperatorId))

	api.logger.Info("Starting server")
	err := router.Run(":8080")
	if err != nil {
		api.logger.Fatal("Error starting server", zap.Error(err))
	}
}

func (api *Api) GetNodes(c *gin.Context) {
	nodes, err := api.db.ListNodeData()
	if err != nil {
		api.logger.Error("Error getting nodes", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	// Add metadata to response
	response := gin.H{
		"nodes": nodes,
		"metadata": gin.H{
			"count": len(nodes),
		},
	}
	c.JSON(http.StatusOK, response)
}

func (api *Api) GetNodeByPubKey(c *gin.Context) {
	pubkey := c.Param("pubkey")
	if pubkey == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "pubkey is required"})
		return
	}
	nodeData, err := api.db.GetNodeData(format.OperatorID(pubkey))
	if err != nil {
		api.logger.Error("Error getting node", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, nodeData)
}

func (api *Api) GetNodeByOperatorId(c *gin.Context) {
	operatodId := c.Param("operatorid")
	if operatodId == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "operatorid is required"})
		return
	}
	nodeData, err := api.db.GetNodeData(operatodId)
	if err != nil {
		api.logger.Error("Error getting node", zap.Error(err))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, nodeData)
}
