package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/stakestar/startracker/db"
	"go.uber.org/zap"
)

type Api struct {
	db *db.BoltDB

	logger *zap.Logger
}

func New(logger *zap.Logger, db *db.BoltDB) *Api {
	return &Api{
		db:     db,
		logger: logger,
	}
}

func (api *Api) Start() {
	router := mux.NewRouter()
	router.HandleFunc("/api/nodes", api.GetNodes).Methods("GET")

	api.logger.Info("Starting server")
	err := http.ListenAndServe(":8080", router)
	if err != nil {
		api.logger.Fatal("Error starting server", zap.Error(err))
	}
}

func (api *Api) GetNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := api.db.ListNodeData()
	if err != nil {
		api.logger.Error("Error getting nodes", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	api.logger.Info("Returning nodes", zap.Any("nodes count", len(nodes)))
	json.NewEncoder(w).Encode(nodes)
}
