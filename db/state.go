package db

import (
	"errors"
	"math/big"

	bolt "go.etcd.io/bbolt"
)

var stateBucketName = []byte("State")

func setupStateBucket(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(stateBucketName)
		return err
	})
}

func (db *BoltDB) GetLastBlock() (*big.Int, error) {
	var data = new(big.Int)
	err := db.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateBucketName)
		value := bucket.Get([]byte("lastBlock"))
		if value == nil {
			return nil
		}
		var ok bool
		data, ok = data.SetString(string(value), 10)
		if !ok {
			return errors.New("error parsing last block number")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (db *BoltDB) SaveLastBlock(blockNumber *big.Int) error {
	key := []byte("lastBlock")
	return db.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateBucketName)
		return bucket.Put(key, []byte(blockNumber.String()))
	})
}
