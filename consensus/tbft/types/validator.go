package types

import (
	"bytes"
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/crypto"
)

// Volatile state for each Validator
// NOTE: The Accum is not included in Validator.Hash();
// make sure to update that method if changes are made here
type Validator struct {
	Address     help.Address       	`json:"address"`
	PubKey      crypto.PubKey 		`json:"pub_key"`
	VotingPower uint64         		`json:"voting_power"`
	Accum 		uint64 				`json:"accum"`
}

func NewValidator(pubKey crypto.PubKey, votingPower uint64) *Validator {
	return &Validator{
		Address:     pubKey.Address(),
		PubKey:      pubKey,
		VotingPower: votingPower,
		Accum:       0,
	}
}

// Creates a new copy of the validator so we can mutate accum.
// Panics if the validator is nil.
func (v *Validator) Copy() *Validator {
	vCopy := *v
	return &vCopy
}

// Returns the one with higher Accum.
func (v *Validator) CompareAccum(other *Validator) *Validator {
	if v == nil {
		return other
	}
	if v.Accum > other.Accum {
		return v
	} else if v.Accum < other.Accum {
		return other
	} else {
		result := bytes.Compare(v.Address, other.Address)
		if result < 0 {
			return v
		} else if result > 0 {
			return other
		} else {
			help.PanicSanity("Cannot compare identical validators")
			return nil
		}
	}
}

func (v *Validator) String() string {
	if v == nil {
		return "nil-Validator"
	}
	return fmt.Sprintf("Validator{%v %v VP:%v A:%v}",
		v.Address,
		v.PubKey,
		v.VotingPower,
		v.Accum)
}

// Hash computes the unique ID of a validator with a given voting power.
// It excludes the Accum value, which changes with every round.
func (v *Validator) Hash() []byte {
	tmp := help.RlpHash(struct {
		Address     help.Address
		PubKey      crypto.PubKey
		VotingPower uint64
	}{
		v.Address,
		v.PubKey,
		v.VotingPower,
	})

	return tmp[:]
}

//----------------------------------------
// RandValidator

// RandValidator returns a randomized validator, useful for testing.
// UNSTABLE
//func RandValidator(randPower bool, minPower int64) (*Validator, PrivValidator) {
//	privVal := NewMockPV()
//	votePower := minPower
//	if randPower {
//		// update by iceming
//		// votePower += int64(cmn.RandUint32())
//		random := rand.New(rand.NewSource(time.Now().Unix()))
//		rdata := random.Uint32()
//		votePower += int64(rdata)
//	}
//	val := NewValidator(privVal.GetPubKey(), votePower)
//	return val, privVal
//}
