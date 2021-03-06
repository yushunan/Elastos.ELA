package contract

import (
	"crypto/sha256"
	"errors"

	"github.com/elastos/Elastos.ELA/common"
	"github.com/elastos/Elastos.ELA/crypto"
	"github.com/elastos/Elastos.ELA/vm"

	"golang.org/x/crypto/ripemd160"
)

type PrefixType byte

const (
	PrefixStandard   PrefixType = 0x21
	PrefixMultiSig   PrefixType = 0x12
	PrefixCrossChain PrefixType = 0x4B
	PrefixDeposit    PrefixType = 0x1F
)

// Contract include the redeem script and hash prefix
type Contract struct {
	Code       []byte
	HashPrefix PrefixType
}

func (c *Contract) ToProgramHash() (*common.Uint168, error) {
	code := c.Code
	if len(code) < 1 {
		return nil, errors.New("[ToProgramHash] failed, empty program code")
	}

	// Check code
	switch code[len(code)-1] {
	case vm.CHECKSIG:
		if len(code) != crypto.PublicKeyScriptLength {
			return nil, errors.New("[ToProgramHash] error, not a valid checksig script")
		}
	case vm.CHECKMULTISIG:
		if len(code) < crypto.MinMultiSignCodeLength || (len(code)-3)%(crypto.PublicKeyScriptLength-1) != 0 {
			return nil, errors.New("[ToProgramHash] error, not a valid multisig script")
		}
	case common.CROSSCHAIN: // FIXME should not use this opcode in future
	default:
		return nil, errors.New("[ToProgramHash] error, unknown opcode")
	}

	hash := sha256.Sum256(code)
	md160 := ripemd160.New()
	md160.Write(hash[:])
	programBytes := md160.Sum([]byte{byte(c.HashPrefix)})

	return common.Uint168FromBytes(programBytes)
}

func (c *Contract) ToCodeHash() *common.Uint160 {
	return common.ToCodeHash(c.Code)
}
