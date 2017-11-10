// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"math/rand"
	"testing"
)

// TestSchnorrThreshold test Schnorr threshold signature
func TestSchnorrThreshold(t *testing.T) {
	const MAX_SIGNATORIES = 10
	const NUM_TEST = 5

	tRand := rand.New(rand.NewSource(543212345))
	numSignatories := MAX_SIGNATORIES * NUM_TEST

	curve := new(TwistedEdwardsCurve)
	curve.InitParam25519()

	msg, _ := hex.DecodeString(
		"d04b98f48e8f8bcc15c6ae5ac050801cd6dcfd428fb5f9e65c4e16e7807340fa")
	seckeys := mockUpSecKeysByScalars(curve, numSignatories)

	for i := 0; i < NUM_TEST; i++ {
		numKeysForTest := tRand.Intn(MAX_SIGNATORIES-2) + 2
		keyIndex := i * MAX_SIGNATORIES

		// retrieve keys fro the faked secret key vector
		keysToUse := make([]*PrivateKey, numKeysForTest, numKeysForTest)
		for j := 0; j < numKeysForTest; j++ {
			keysToUse[j] = seckeys[j+keyIndex]
		}

		// compute the corresponding public key vector
		pubKeysToUse := make([]*PublicKey, numKeysForTest, numKeysForTest)
		for j := 0; j < numKeysForTest; j++ {
			_, pubKeysToUse[j], _ = PrivKeyFromScalar(curve, keysToUse[j].Serialize())
		}

		// Combine pubkeys.
		allPubkeys := make([]*PublicKey, numKeysForTest)
		copy(allPubkeys, pubKeysToUse)

		allPksSum := CombinePubkeys(curve, allPubkeys)

		// fake the secret and public nonce vectors
		privNoncesToUse := make([]*PrivateKey, numKeysForTest, numKeysForTest)
		pubNoncesToUse := make([]*PublicKey, numKeysForTest, numKeysForTest)
		for j := 0; j < numKeysForTest; j++ {
			nonce := nonceRFC6979(curve, keysToUse[j].Serialize(), msg, nil,
				Sha512VersionStringRFC6979)
			nonceBig := new(big.Int).SetBytes(nonce)
			nonceBig.Mod(nonceBig, curve.N)
			nonce = copyBytes(nonceBig.Bytes())[:]
			nonce[31] &= 248

			privNonce, pubNonce, err := PrivKeyFromScalar(curve, nonce[:])
			if err != nil {
				t.Fatalf("unexpected error %s, ", err)
			}
			if privNonce == nil {
				t.Fatalf("private nonce shouldn't be nil")
			}

			if pubNonce == nil {
				t.Fatalf("public nonce shouldn't be nil")
			}

			privNoncesToUse[j] = privNonce
			pubNoncesToUse[j] = pubNonce
		}

		partialSignatures := make([]*Signature, numKeysForTest, numKeysForTest)

		// Partial signature generation.
		publicNonceSum := CombinePubkeys(curve, pubNoncesToUse)
		if publicNonceSum == nil {
			t.Fatal("sum of public nonce should be nonzero")
		}
		for j := range keysToUse {
			r, s, err := schnorrPartialSign(curve, msg, keysToUse[j].Serialize(),
				allPksSum.Serialize(), privNoncesToUse[j].Serialize(),
				publicNonceSum.Serialize())
			if err != nil {
				t.Fatalf("unexpected error %s, ", err)
			}

			localSig := NewSignature(r, s)
			partialSignatures[j] = localSig
		}

		// Combine signatures.
		combinedSignature, err := SchnorrCombineSigs(curve, partialSignatures)
		if err != nil {
			t.Fatalf("unexpected error %s, ", err)
		}

		// Make sure the combined signatures are the same as the
		// signatures that would be generated by simply adding
		// the private keys and private nonces.
		combinedPrivkeysD := new(big.Int).SetInt64(0)
		for _, priv := range keysToUse {
			combinedPrivkeysD = ScalarAdd(combinedPrivkeysD, priv.GetD())
			combinedPrivkeysD = combinedPrivkeysD.Mod(combinedPrivkeysD, curve.N)
		}

		combinedNonceD := new(big.Int).SetInt64(0)
		for _, priv := range privNoncesToUse {
			combinedNonceD.Add(combinedNonceD, priv.GetD())
			combinedNonceD.Mod(combinedNonceD, curve.N)
		}

		// convert the scalar to a valid secret key for curve
		combinedPrivkey, _, err := PrivKeyFromScalar(curve,
			copyBytes(combinedPrivkeysD.Bytes())[:])
		if err != nil {
			t.Fatalf("unexpected error %s, ", err)
		}
		// convert the scalar to a valid nonce for curve
		combinedNonce, _, err := PrivKeyFromScalar(curve,
			copyBytes(combinedNonceD.Bytes())[:])
		if err != nil {
			t.Fatalf("unexpected error %s, ", err)
		}

		// sign with the combined secret key and nonce
		cSigR, cSigS, err := SignFromScalar(curve, combinedPrivkey,
			combinedNonce.Serialize(), msg)
		sumSig := NewSignature(cSigR, cSigS)
		if err != nil {
			t.Fatalf("unexpected error %s, ", err)
		}
		if !bytes.Equal(sumSig.Serialize(), combinedSignature.Serialize()) {
			t.Fatalf("want %s, got %s",
				hex.EncodeToString(combinedSignature.Serialize()),
				hex.EncodeToString(sumSig.Serialize()))
		}

		// Verify the combined signature and public keys.
		if !Verify(allPksSum, msg, combinedSignature.GetR(),
			combinedSignature.GetS()) {
			t.Fatalf("failed to verify the combined signature")
		}

		// Corrupt some memory and make sure it breaks something.
		//corruptWhat := tRand.Intn(4)
		corruptWhat := 1
		randItem := tRand.Intn(numKeysForTest - 1)

		// Corrupt private key.
		if corruptWhat == 0 {
			privSerCorrupt := keysToUse[randItem].Serialize()
			pos := tRand.Intn(31)
			bitPos := tRand.Intn(7)
			privSerCorrupt[pos] ^= 1 << uint8(bitPos)
			keysToUse[randItem].ecPk.D.SetBytes(privSerCorrupt)
		}
		// Corrupt public key.
		if corruptWhat == 1 {
			pubXCorrupt := BigIntToEncodedBytes(pubKeysToUse[randItem].GetX())
			pos := tRand.Intn(31)
			bitPos := tRand.Intn(7)
			pubXCorrupt[pos] ^= 1 << uint8(bitPos)
			pubKeysToUse[randItem].GetX().SetBytes(pubXCorrupt[:])
		}
		// Corrupt private nonce.
		if corruptWhat == 2 {
			privSerCorrupt := privNoncesToUse[randItem].Serialize()
			pos := tRand.Intn(31)
			bitPos := tRand.Intn(7)
			privSerCorrupt[pos] ^= 1 << uint8(bitPos)
			privNoncesToUse[randItem].ecPk.D.SetBytes(privSerCorrupt)
		}
		// Corrupt public nonce.
		if corruptWhat == 3 {
			pubXCorrupt := BigIntToEncodedBytes(pubNoncesToUse[randItem].GetX())
			pos := tRand.Intn(31)
			bitPos := tRand.Intn(7)
			pubXCorrupt[pos] ^= 1 << uint8(bitPos)
			pubNoncesToUse[randItem].GetX().SetBytes(pubXCorrupt[:])
		}

		for j := range keysToUse {
			thisPubNonce := pubNoncesToUse[j]
			localPubNonces := make([]*PublicKey, numKeysForTest-1, numKeysForTest-1)
			itr := 0
			for _, pubNonce := range pubNoncesToUse {
				if bytes.Equal(thisPubNonce.Serialize(), pubNonce.Serialize()) {
					continue
				}
				localPubNonces[itr] = pubNonce
				itr++
			}
			publicNonceSum := CombinePubkeys(curve, localPubNonces)

			sigR, sigS, _ := schnorrPartialSign(curve, msg,
				keysToUse[j].Serialize(), allPksSum.Serialize(),
				privNoncesToUse[j].Serialize(),
				publicNonceSum.Serialize())
			localSig := NewSignature(sigR, sigS)

			partialSignatures[j] = localSig
		}

		// Combine signatures.
		combinedSignature, _ = SchnorrCombineSigs(curve, partialSignatures)

		// Nothing that makes it here should be valid.
		if allPksSum != nil && combinedSignature != nil {
			if Verify(allPksSum, msg, combinedSignature.GetR(),
				combinedSignature.GetS()) {
				t.Fatal("failed to verify combined signature")
			}
		}
	}
}
