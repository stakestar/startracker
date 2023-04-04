package db

import (
	"encoding/json"

	"github.com/stakestar/startracker/utils"
	bolt "go.etcd.io/bbolt"
)

var operatorsBucketName = []byte("Operators")
var operatorsContractIdToOperatorIdBucketName = []byte("OperatorsContractIdToOperatorId")

func setupOperatorsBucket(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(operatorsBucketName)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(operatorsContractIdToOperatorIdBucketName)
		return err
	})
}

func (db *BoltDB) SaveOperatorAndUpdateNodeData(operator *Operator) error {
	err := db.SaveOperator(operator)
	if err != nil {
		return err
	}

	err = db.SaveOperatorContractIdToOperatorId(operator.OperatorID, operator.OperatorIDContract)
	if err != nil {
		return err
	}

	nodeData, err := db.GetNodeData(operator.OperatorID)
	if err == nil {
		nodeData.OperatorIDContract = operator.OperatorIDContract
		err = db.StoreNodeData(nodeData)
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *BoltDB) SaveOperator(operator *Operator) error {
	key := []byte(operator.OperatorID)
	value, err := json.Marshal(operator)
	if err != nil {
		return err
	}
	return db.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(operatorsBucketName)
		return bucket.Put(key, value)
	})
}

func (db *BoltDB) SaveOperatorContractIdToOperatorId(operatorId string, operatorIdContract uint64) error {
	key := utils.Uint64ToBytes(operatorIdContract)
	value := []byte(operatorId)
	return db.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(operatorsContractIdToOperatorIdBucketName)
		return bucket.Put(key, value)
	})
}

func (db *BoltDB) GetOperatorByOperatorId(operatorId string) (*Operator, error) {
	var data Operator
	err := db.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(operatorsBucketName)
		value := bucket.Get([]byte(operatorId))
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

func (db *BoltDB) GetOperatorByOperatorIdContract(operatorIdContract uint64) (*Operator, error) {
	var data Operator
	err := db.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(operatorsContractIdToOperatorIdBucketName)
		operatorId := bucket.Get(utils.Uint64ToBytes(operatorIdContract))
		if operatorId == nil {
			return ErrNotFound
		}
		bucket = tx.Bucket(operatorsBucketName)
		value := bucket.Get(operatorId)
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
