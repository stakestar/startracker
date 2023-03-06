package keys

import (
	"crypto/rsa"

	"github.com/bloxapp/ssv/utils/rsaencryption"
)

func GenerateKeys() (*rsa.PrivateKey, string, error) {
	_, skByte, err := rsaencryption.GenerateKeys()
	if err != nil {
		return nil, "", err
	}

	privateKey, err := rsaencryption.ConvertPemToPrivateKey(string(skByte))
	if err != nil {
		return nil, "", err
	}

	operatorPubKey, err := rsaencryption.ExtractPublicKey(privateKey)
	if err != nil {
		return nil, "", err
	}

	return privateKey, operatorPubKey, nil
}
