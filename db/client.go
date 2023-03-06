package db

import (
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"
)

type BoltDB struct {
	db *bolt.DB
}

var nodeDataBucketName = []byte("NodeData")

func NewBoltDB(dbPath string) (*BoltDB, error) {
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(nodeDataBucketName)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &BoltDB{db}, nil
}

func (db *BoltDB) Close() error {
	return db.db.Close()
}

func (db *BoltDB) StoreNodeData(data *NodeData) error {
	data.UpdatedAt = time.Now()
	key := []byte(data.OperatorID)
	value, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return db.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(nodeDataBucketName)
		return bucket.Put(key, value)
	})
}

func (db *BoltDB) ListNodeData() ([]NodeData, error) {
	var dataList []NodeData
	err := db.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(nodeDataBucketName)
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var data NodeData
			err := json.Unmarshal(v, &data)
			if err != nil {
				return err
			}
			dataList = append(dataList, data)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dataList, nil
}
