package parser

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/envkey/envkey-fetch/crypto"
	"github.com/envkey/envkey-fetch/trust"
	"github.com/envkey/myhttp"

	"golang.org/x/crypto/openpgp"
)

type EnvServiceResponse struct {
	Env                                        string `json:"env"`
	EncryptedPrivkey                           string `json:"encrypted_privkey"`
	PubkeyArmored                              string `json:"pubkey"`
	SignedTrustedPubkeys                       string `json:"signed_trusted_pubkeys"`
	SignedById                                 string `json:"signed_by_id"`
	SignedByPubkeyArmored                      string `json:"signed_by_pubkey"`
	SignedByTrustedPubkeys                     string `json:"signed_by_trusted_pubkeys"`
	InheritanceOverrides                       string `json:"inheritance_overrides,omitempty"`
	InheritanceOverridesSignedById             string `json:"inheritance_overrides_signed_by_id,omitempty"`
	InheritanceOverridesSignedByPubkeyArmored  string `json:"inheritance_overrides_signed_by_pubkey,omitempty"`
	InheritanceOverridesSignedByTrustedPubkeys string `json:"inheritance_overrides_signed_by_trusted_pubkeys,omitempty"`
	AllowCaching                               bool   `json:"allow_caching"`
}

func (response *EnvServiceResponse) validate() error {
	valid := response.Env != "" &&
		response.EncryptedPrivkey != "" &&
		response.PubkeyArmored != "" &&
		response.SignedTrustedPubkeys != "" &&
		response.SignedById != "" &&
		response.SignedByPubkeyArmored != "" &&
		response.SignedByTrustedPubkeys != ""

	if !valid {
		return errors.New("Required fields are empty.")
	}

	return response.validateInheritanceOverrides()
}

func (response *EnvServiceResponse) hasInheritanceOverrides() bool {
	return response.InheritanceOverrides != "" &&
		response.InheritanceOverridesSignedById != "" &&
		response.InheritanceOverridesSignedByPubkeyArmored != "" &&
		response.InheritanceOverridesSignedByTrustedPubkeys != ""
}

func (response *EnvServiceResponse) validateInheritanceOverrides() error {
	hasAnyFields := response.InheritanceOverrides != "" ||
		response.InheritanceOverridesSignedById != "" ||
		response.InheritanceOverridesSignedByPubkeyArmored != "" ||
		response.InheritanceOverridesSignedByTrustedPubkeys != ""

	if hasAnyFields && !response.hasInheritanceOverrides() {
		return errors.New("Invalid inheritance override fields.")
	}

	return nil
}

func (response *EnvServiceResponse) Parse(pw string) (string, error) {
	var err error
	var responseWithKeys *ResponseWithKeys
	var responseWithTrustChain *ResponseWithTrustChain
	var decryptedVerified *DecryptedVerifiedResponse

	err = response.validate()
	if err != nil {
		return "", err
	}

	responseWithKeys, err = response.parseKeys(pw)
	if err != nil {
		return "", err
	}

	responseWithTrustChain, err = responseWithKeys.parseTrustChain()
	if err != nil {
		return "", err
	}

	decryptedVerified, err = responseWithTrustChain.decryptAndVerify()
	if err != nil {
		return "", err
	}

	return decryptedVerified.toJson()
}

func (response *EnvServiceResponse) parseKeys(pw string) (*ResponseWithKeys, error) {
	var err error
	var decryptedPrivkey, verifiedPubkey, signedByPubkey, inheritanceOverridesSignedByPubkey openpgp.EntityList

	decryptedPrivkey, err = crypto.ReadPrivkey([]byte(response.EncryptedPrivkey), []byte(pw))
	if err != nil {
		return nil, err
	}

	verifiedPubkey, err = crypto.ReadArmoredKey([]byte(response.PubkeyArmored))
	if err != nil {
		return nil, err
	}

	err = crypto.VerifyPubkeyWithPrivkey(verifiedPubkey, decryptedPrivkey)
	if err != nil {
		return nil, err
	}

	signedByPubkey, err = crypto.ReadArmoredKey([]byte(response.SignedByPubkeyArmored))
	if err != nil {
		return nil, err
	}

	if response.hasInheritanceOverrides() {
		inheritanceOverridesSignedByPubkey, err = crypto.ReadArmoredKey([]byte(response.InheritanceOverridesSignedByPubkeyArmored))
		if err != nil {
			return nil, err
		}
	}

	responseWithKeys := ResponseWithKeys{
		RawResponse:                        response,
		DecryptedPrivkey:                   decryptedPrivkey,
		VerifiedPubkey:                     verifiedPubkey,
		SignerKeyring:                      append(decryptedPrivkey, signedByPubkey...),
		SignedByPubkey:                     signedByPubkey,
		InheritanceOverridesSignedByPubkey: inheritanceOverridesSignedByPubkey,
		InheritanceSignerKeyring:           append(decryptedPrivkey, inheritanceOverridesSignedByPubkey...),
	}

	return &responseWithKeys, nil
}

type ResponseWithKeys struct {
	RawResponse                                                                                                                   *EnvServiceResponse
	DecryptedPrivkey, VerifiedPubkey, SignerKeyring, InheritanceSignerKeyring, SignedByPubkey, InheritanceOverridesSignedByPubkey openpgp.EntityList
}

func (response *ResponseWithKeys) hasInheritanceOverrides() bool {
	return response.RawResponse.hasInheritanceOverrides()
}

func (response *ResponseWithKeys) signer() *trust.Signer {
	return &trust.Signer{
		response.RawResponse.SignedById,
		response.RawResponse.SignedByPubkeyArmored,
		response.SignedByPubkey,
		false,
	}
}

func (response *ResponseWithKeys) inheritanceOverridesSigner() *trust.Signer {
	if !response.hasInheritanceOverrides() {
		return nil
	}
	return &trust.Signer{
		response.RawResponse.InheritanceOverridesSignedById,
		response.RawResponse.InheritanceOverridesSignedByPubkeyArmored,
		response.InheritanceOverridesSignedByPubkey,
		true,
	}
}

func (response *ResponseWithKeys) trustedKeyablesChain() (*trust.TrustedKeyablesChain, error) {
	var err error
	var creatorTrusted, signerTrusted, inheritanceOverridesTrusted trust.TrustedKeyablesMap

	creatorTrusted, err = parseTrustedKeys(response.RawResponse.SignedTrustedPubkeys, response.VerifiedPubkey)
	if err != nil {
		return nil, err
	}

	signerTrusted, err = parseTrustedKeys(response.RawResponse.SignedByTrustedPubkeys, response.SignedByPubkey)
	if err != nil {
		return nil, err
	}

	if response.hasInheritanceOverrides() {
		inheritanceOverridesTrusted, err = parseTrustedKeys(
			response.RawResponse.InheritanceOverridesSignedByTrustedPubkeys,
			response.InheritanceOverridesSignedByPubkey,
		)
		if err != nil {
			return nil, err
		}
	}

	trustedChain := trust.TrustedKeyablesChain{creatorTrusted, signerTrusted, inheritanceOverridesTrusted}

	return &trustedChain, nil
}

func (response *ResponseWithKeys) parseTrustChain() (*ResponseWithTrustChain, error) {
	trustedKeyablesChain, err := response.trustedKeyablesChain()
	if err != nil {
		return nil, err
	}

	responseWithTrustChain := ResponseWithTrustChain{
		ResponseWithKeys:     response,
		TrustedKeyablesChain: trustedKeyablesChain,
		Signer:               response.signer(),
		InheritanceOverridesSigner: response.inheritanceOverridesSigner(),
	}

	return &responseWithTrustChain, nil
}

type ResponseWithTrustChain struct {
	ResponseWithKeys           *ResponseWithKeys
	TrustedKeyablesChain       *trust.TrustedKeyablesChain
	Signer                     *trust.Signer
	InheritanceOverridesSigner *trust.Signer
}

func (response *ResponseWithTrustChain) hasInheritanceOverrides() bool {
	return response.ResponseWithKeys.hasInheritanceOverrides()
}

func (response *ResponseWithTrustChain) verifyTrusted(signer *trust.Signer) error {
	trusted, _, err := response.TrustedKeyablesChain.SignerTrustedKeyable(signer)

	if err != nil {
		return err
	} else if trusted == nil {
		return errors.New("Signer not trusted.")
	}

	return nil
}

func (response *ResponseWithTrustChain) decryptAndVerify() (*DecryptedVerifiedResponse, error) {
	var err error

	// verify signer trusted
	err = response.verifyTrusted(response.Signer)
	if err != nil {
		return nil, err
	}

	// verify inheritance overrides signer trusted
	if response.hasInheritanceOverrides() {
		err = response.verifyTrusted(response.InheritanceOverridesSigner)
		if err != nil {
			return nil, err
		}
	}

	decryptedVerifiedResponse := new(DecryptedVerifiedResponse)
	decryptedVerifiedResponse.ResponseWithTrustChain = response

	decryptedVerifiedResponse.decryptEnv()
	decryptedVerifiedResponse.checkEnvUrlPointer()

	if response.hasInheritanceOverrides() {
		decryptedVerifiedResponse.decryptInheritanceOverrides()
		decryptedVerifiedResponse.checkInheritanceOverridesUrlPointer()

		var env, inheritanceOverrides map[string]interface{}
		err = json.Unmarshal(decryptedEnvBytes, &env)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(decryptedInheritanceBytes, &inheritanceOverrides)
		if err != nil {
			return nil, err
		}

		decryptedVerifiedResponse.DecryptedEnv = env
		decryptedVerifiedResponse.DecryptedInheritanceOverrides = inheritanceOverrides
	}

	return decryptedVerifiedResponse, nil
}

type DecryptedVerifiedResponse struct {
	ResponseWithTrustChain              *ResponseWithTrustChain
	DecryptedEnvBytes                   []byte
	DecryptedInheritanceOverridesBytes  []byte
	DecryptedEnvString                  string
	DecryptedInheritanceOverridesString string
	DecryptedEnv                        map[string]interface{}
	DecryptedInheritanceOverrides       map[string]interface{}
}

func (response *DecryptedVerifiedResponse) decryptEnv() error {
	if response.ResponseWithTrustChain == nil {
		return errors.New("ResponseWithTrustChain required for decryption.")
	}

	// decrypt env
	response.DecryptedEnvBytes, err = crypto.DecryptAndVerify(
		[]byte(response.ResponseWithTrustChain.ResponseWithKeys.RawResponse.Env),
		response.ResponseWithTrustChain.ResponseWithKeys.SignerKeyring,
	)
	if err != nil {
		return nil, err
	}

	response.DecryptedEnvString = string(decryptedEnvBytes)

	return nil
}

func (response *DecryptedVerifiedResponse) decryptInheritanceOverrides() error {
	reponse.DecryptedInheritanceBytes, err = crypto.DecryptAndVerify(
		[]byte(response.ResponseWithKeys.RawResponse.InheritanceOverrides),
		response.ResponseWithKeys.InheritanceSignerKeyring,
	)
	if err != nil {
		return nil, err
	}

	response.DecryptedInheritanceOverridesString = string(response.DecryptedInheritanceBytes)
}

func (response *DecryptedVerifiedResponse) checkEnvUrlPointer() error {
	if response.DecryptedEnvString == "" {
		return errors.New("env must first be decrypted before checking for url pointer.")
	}

	// if decrypted env is a simple string (not an object), treat as url pointer
	if !strings.HasPrefix(decryptedVerified.DecryptedEnvString, "{") {
		var err error
		var body []byte
		var r *http.Response

		url := decryptedVerified.DecryptedEnvString

		getter := myhttp.New(time.Second * 2)
		r, err = getter.Get(url)
		if r != nil {
			defer r.Body.Close()
		}
		if err != nil {
			return err
		} else if r.StatusCode >= 400 {
			return errors.New("environment pointer url could not be loaded.")
		}

		body, err = ioutil.ReadAll(r.Body)

		if err != nil {
			return err
		}

		response.ResponseWithTrustChain.ResponseWithKeys.RawResponse.env = body
		return response.decryptEnv()
	}
}

func (response *DecryptedVerifiedResponse) checkInheritanceOverridesUrlPointer() error {
	if response.DecryptedInheritanceOverridesString == "" {
		return errors.New("inheritance overrides must first be decrypted before checking for url pointer.")
	}

	// if decrypted env is a simple string (not an object), treat as url pointer
	if !strings.HasPrefix(decryptedVerified.DecryptedInheritanceOverridesString, "{") {
		var err error
		var body []byte
		var r *http.Response

		url := decryptedVerified.DecryptedInheritanceOverridesString

		getter := myhttp.New(time.Second * 2)
		r, err = getter.Get(url)
		if r != nil {
			defer r.Body.Close()
		}
		if err != nil {
			return err
		} else if r.StatusCode >= 400 {
			return errors.New("environment pointer url could not be loaded.")
		}

		body, err = ioutil.ReadAll(r.Body)

		if err != nil {
			return err
		}

		response.ResponseWithTrustChain.ResponseWithKeys.RawResponse.env = body
		return response.decryptEnv()
	}

}

func (response *DecryptedVerifiedResponse) toJson() (string, error) {
	if response.DecryptedEnvString == "" {
		envJson, err := json.Marshal(response.toMap())
		if err != nil {
			return "", err
		}
		return string(envJson), nil
	} else {
		return response.DecryptedEnvString, nil
	}
}

func (response *DecryptedVerifiedResponse) toMap() map[string]interface{} {
	if response.DecryptedEnvString != "" || response.DecryptedEnv == nil || response.DecryptedInheritanceOverrides == nil {
		return nil
	}
	env := response.DecryptedEnv
	for k, v := range response.DecryptedInheritanceOverrides {
		env[k] = v
	}
	return env
}

func parseTrustedKeys(rawTrusted string, signerPubkey openpgp.EntityList) (trust.TrustedKeyablesMap, error) {
	var err error
	var verified []byte

	trusted := make(trust.TrustedKeyablesMap)

	verified, err = crypto.VerifySignedCleartext([]byte(rawTrusted), signerPubkey)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(verified, &trusted)
	if err != nil {
		return nil, err
	}

	return trusted, nil
}
