package trust

import (
	"errors"

	"github.com/envkey/envkey-fetch/crypto"

	"golang.org/x/crypto/openpgp"
)

type Signer struct {
	Id                  string
	PubkeyArmored       string
	Pubkey              openpgp.EntityList
	IsInheritanceSigner bool
}

func NewSigner(id, pubkeyArmored string, isInheritanceSigner bool) (*Signer, error) {
	pubkey, err := crypto.ReadArmoredKey([]byte(pubkeyArmored))
	if err != nil {
		return nil, err
	}
	return &Signer{id, pubkeyArmored, pubkey, isInheritanceSigner}, nil
}

type TrustedKeyable struct {
	PubkeyArmored       string `json:"pubkey"`
	InvitePubkeyArmored string `json:"invitePubkey,omitempty"`
	InvitedById         string `json:"invitedById,omitempty"`
}

func (keyable *TrustedKeyable) VerifyInviter(inviterKeyable *TrustedKeyable) error {
	// Verify signed key signature
	pubkeyArmored := keyable.PubkeyArmored
	invitePubkeyArmored := keyable.InvitePubkeyArmored
	inviterPubkeyArmored := inviterKeyable.PubkeyArmored

	err := crypto.VerifyPubkeyArmoredSignature([]byte(invitePubkeyArmored), []byte(inviterPubkeyArmored))
	if err != nil {
		return err
	}

	// If invite, further verify that pubkey was signed by invite key
	return crypto.VerifyPubkeyArmoredSignature([]byte(pubkeyArmored), []byte(invitePubkeyArmored))
}

type TrustedKeyablesMap map[string]TrustedKeyable

func (trustedKeyables TrustedKeyablesMap) SignerTrustedKeyable(signer *Signer) (*TrustedKeyable, error) {
	if trusted, ok := trustedKeyables[signer.Id]; ok {
		trustedPubkey, err := crypto.ReadArmoredKey([]byte(trusted.PubkeyArmored))
		if err != nil {
			return nil, err
		}

		if trustedPubkey[0].PrimaryKey.Fingerprint == signer.Pubkey[0].PrimaryKey.Fingerprint {
			return &trusted, nil
		} else {
			return nil, errors.New("Signer pubkey fingerprint does not match trusted pubkey fingerprint.")
		}
	} else {
		return nil, nil
	}
}

func (trustedKeyables TrustedKeyablesMap) TrustedRoot(keyable *TrustedKeyable, creatorTrusted TrustedKeyablesMap) ([]*TrustedKeyable, error) {
	var trustedRoot *TrustedKeyable
	var newlyVerified []*TrustedKeyable
	var ok bool
	currentKeyable := keyable
	checked := make(map[string]bool)

	for trustedRoot == nil {
		if currentKeyable.InvitedById == "" {
			return nil, errors.New("No signing id.")
		}

		if _, ok = checked[currentKeyable.InvitedById]; ok {
			return nil, errors.New("Already checked signing id: " + currentKeyable.InvitedById)
		}

		var inviterKeyable TrustedKeyable
		inviterKeyable, ok = creatorTrusted[currentKeyable.InvitedById]
		if ok {
			trustedRoot = &inviterKeyable
		} else {
			inviterKeyable, ok = trustedKeyables[currentKeyable.InvitedById]
			if !ok {
				return nil, errors.New("No trusted root.")
			}
		}

		err := currentKeyable.VerifyInviter(&inviterKeyable)
		if err != nil {
			return nil, err
		}

		// currentKeyable now verified
		checked[currentKeyable.InvitedById] = true
		newlyVerified = append(newlyVerified, currentKeyable)

		if trustedRoot == nil {
			currentKeyable = &inviterKeyable
		}
	}

	if trustedRoot == nil {
		return nil, errors.New("No trusted root.")
	}

	return newlyVerified, nil
}

type TrustedKeyablesChain struct {
	CreatorTrusted                    TrustedKeyablesMap
	SignerTrusted                     TrustedKeyablesMap
	InheritanceOverridesSignerTrusted TrustedKeyablesMap
}

func (trustedKeyables *TrustedKeyablesChain) VerifySignerTrusted(signer *Signer) error {
	_, _, err := trustedKeyables.SignerTrustedKeyable(signer)
	return err
}

func (trustedKeyables *TrustedKeyablesChain) SignerTrustedKeyable(signer *Signer) (*TrustedKeyable, []*TrustedKeyable, error) {
	var err error
	var trusted *TrustedKeyable
	var newlyVerified []*TrustedKeyable

	// First check if key is present in CreatorTrusted keys, which means it's trusted, so we can return
	trusted, err = trustedKeyables.CreatorTrusted.SignerTrustedKeyable(signer)
	if err != nil {
		return nil, nil, err
	} else if trusted != nil {
		return trusted, []*TrustedKeyable{}, nil
	}

	if signer.IsInheritanceSigner {
		if trustedKeyables.InheritanceOverridesSignerTrusted == nil {
			return nil, nil, errors.New("Inheritance overrides signer not trusted.")
		}

		// If inheritance overrides signer, find key in InheritanceOverridesSignerTrusted
		trusted, err = trustedKeyables.InheritanceOverridesSignerTrusted.SignerTrustedKeyable(signer)
		if err != nil {
			return nil, nil, err
		} else if trusted == nil {
			return nil, nil, errors.New("Inheritance overrides signer not trusted.")
		}

		// Then attempt to validate trust chain back to a CreatorTrusted key
		newlyVerified, err = trustedKeyables.InheritanceOverridesSignerTrusted.TrustedRoot(trusted, trustedKeyables.CreatorTrusted)
		if err != nil {
			return nil, nil, err
		}

	} else {
		// If env signer, find key in InheritanceOverridesSignerTrusted (checking only InheritanceOverridesSignerTrusted keys)
		trusted, err = trustedKeyables.SignerTrusted.SignerTrustedKeyable(signer)
		if err != nil {
			return nil, nil, err
		} else if trusted == nil {
			return nil, nil, errors.New("Signer not trusted.")
		}

		// Then attempt to validate trust chain back to a CreatorTrusted key (checking only SignerTrusted keys)
		newlyVerified, err = trustedKeyables.SignerTrusted.TrustedRoot(trusted, trustedKeyables.CreatorTrusted)
		if err != nil {
			return nil, nil, err
		}
	}

	return trusted, newlyVerified, nil
}
