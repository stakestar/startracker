package db

import (
	"encoding/json"
	"time"

	"github.com/stakestar/startracker/utils"
	bolt "go.etcd.io/bbolt"
)

var nodeDataBucketName = []byte("NodeData")

func setupNodeDataBucket(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(nodeDataBucketName)
		return err
	})
}

func (db *BoltDB) StoreNodeData(data *NodeData) error {
	saveData, err := db.GetNodeData(data.OperatorID)
	if err != nil && err != ErrNotFound {
		return err
	}

	if saveData != nil && saveData.OperatorIDContract != 0 {
		data.OperatorIDContract = saveData.OperatorIDContract
	} else {
		operator, err := db.GetOperatorByOperatorId(data.OperatorID)
		if err != nil && err != ErrNotFound {
			return err
		}
		if operator != nil {
			data.OperatorIDContract = operator.OperatorIDContract
		}
	}

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

func (db *BoltDB) ListNodeData(onlyWithNotNilOperatorId bool) ([]NodeData, error) {
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

			if onlyWithNotNilOperatorId && data.OperatorIDContract == 0 {
				continue
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

func (db *BoltDB) GetNodeData(operatorID string) (*NodeData, error) {
	var data NodeData
	err := db.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(nodeDataBucketName)
		value := bucket.Get([]byte(operatorID))
		if value == nil {
			return ErrNotFound
		}
		return json.Unmarshal(value, &data)
	})
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func (db *BoltDB) GetNodeByOperatorContractId(operatorContractId string) (*NodeData, error) {
	var data NodeData
	err := db.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(nodeDataBucketName)
		operatorsContractIdToOperatorIdBucket := tx.Bucket(operatorsContractIdToOperatorIdBucketName)

		key, err := utils.StringUint64ToBytes(operatorContractId)
		if err != nil {
			return err
		}

		operatorID := operatorsContractIdToOperatorIdBucket.Get(key)
		if operatorID == nil {
			return ErrNotFound
		}

		value := bucket.Get([]byte(operatorID))
		if value == nil {
			return ErrNotFound
		}
		return json.Unmarshal(value, &data)
	})
	if err != nil {
		return nil, err
	}
	return &data, nil
}
