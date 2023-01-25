package types

import (
	"errors"

	"github.com/celestiaorg/nmt"
	"github.com/tendermint/tendermint/crypto/merkle"
	tmbytes "github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/pkg/consts"
	"github.com/tendermint/tendermint/proto/tendermint/crypto"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
)

// ShareProof is an NMT proof that a set of shares exist in a set of rows and a
// Merkle proof that those rows exist in a Merkle tree with a given data root.
type ShareProof struct {
	// Data are the raw shares that are being proven.
	Data [][]byte `json:"data"`
	// ShareProofs are NMT proofs that the shares in Data exist in a set of
	// rows. There will be one ShareProof per row that the shares occupy.
	ShareProofs []*tmproto.NMTProof `json:"share_proofs"`
	// NamespaceID is the namespace ID of the shares being proven. This
	// namespace is used when verifying the proof. If the namespace ID doesn't
	// match the namespace of the shares, the proof will fail verification.
	NamespaceID []byte   `json:"namespace_id"`
	RowProof    RowProof `json:"row_proof"`
}

func (sp ShareProof) ToProto() tmproto.ShareProof {
	rowRoots := make([][]byte, len(sp.RowProof.RowRoots))
	rowProofs := make([]*crypto.Proof, len(sp.RowProof.Proofs))
	for i := range sp.RowProof.RowRoots {
		rowRoots[i] = sp.RowProof.RowRoots[i].Bytes()
		rowProofs[i] = sp.RowProof.Proofs[i].ToProto()
	}
	pbtp := tmproto.ShareProof{
		Data:        sp.Data,
		ShareProofs: sp.ShareProofs,
		NamespaceId: sp.NamespaceID,
		RowProof: &tmproto.RowProof{
			RowRoots: rowRoots,
			Proofs:   rowProofs,
			StartRow: sp.RowProof.StartRow,
			EndRow:   sp.RowProof.EndRow,
		},
	}

	return pbtp
}

// ShareProofFromProto creates a ShareProof from a proto message.
// Expects the proof to be pre-validated.
func ShareProofFromProto(pb tmproto.ShareProof) (ShareProof, error) {
	rowRoots := make([]tmbytes.HexBytes, len(pb.RowProof.RowRoots))
	rowProofs := make([]*merkle.Proof, len(pb.RowProof.Proofs))
	for i := range pb.RowProof.Proofs {
		rowRoots[i] = pb.RowProof.RowRoots[i]
		rowProofs[i] = &merkle.Proof{
			Total:    pb.RowProof.Proofs[i].Total,
			Index:    pb.RowProof.Proofs[i].Index,
			LeafHash: pb.RowProof.Proofs[i].LeafHash,
			Aunts:    pb.RowProof.Proofs[i].Aunts,
		}
	}

	return ShareProof{
		RowProof: RowProof{
			RowRoots: rowRoots,
			Proofs:   rowProofs,
			StartRow: pb.RowProof.StartRow,
			EndRow:   pb.RowProof.EndRow,
		},
		Data:        pb.Data,
		ShareProofs: pb.ShareProofs,
		NamespaceID: pb.NamespaceId,
	}, nil
}

// Validate runs basic validations on the proof then verifies if it is consistent.
// It returns nil if the proof is valid. Otherwise, it returns a sensible error.
// The `root` is the block data root that the shares to be proven belong to.
// Note: these proofs are tested on the app side.
func (sp ShareProof) Validate(root []byte) error {
	numberOfSharesInProofs := int32(0)
	for _, proof := range sp.ShareProofs {
		// the range is not inclusive from the left.
		numberOfSharesInProofs += proof.End - proof.Start
	}

	if len(sp.RowProof.RowRoots) != len(sp.ShareProofs) ||
		int32(len(sp.Data)) != numberOfSharesInProofs {
		return errors.New(
			// TODO this should be two separate error messages. The number of
			// row roots does not need to match the number of shares
			"invalid number of proofs, row roots, or data. they all must be the same to verify the proof",
		)
	}

	for _, proof := range sp.ShareProofs {
		if proof.Start < 0 {
			return errors.New("proof index cannot be negative")
		}
		if (proof.End - proof.Start) <= 0 {
			return errors.New("proof total must be positive")
		}
	}

	valid := sp.VerifyProof()
	if !valid {
		return errors.New("proof is not internally consistent")
	}

	if err := sp.RowProof.Validate(root); err != nil {
		return err
	}

	return nil
}

func (sp ShareProof) VerifyProof() bool {
	cursor := int32(0)
	for i, proof := range sp.ShareProofs {
		nmtProof := nmt.NewInclusionProof(
			int(proof.Start),
			int(proof.End),
			proof.Nodes,
			true,
		)
		sharesUsed := proof.End - proof.Start
		valid := nmtProof.VerifyInclusion(
			consts.NewBaseHashFunc(),
			sp.NamespaceID,
			sp.Data[cursor:sharesUsed+cursor],
			sp.RowProof.RowRoots[i],
		)
		if !valid {
			return false
		}
		cursor += sharesUsed
	}
	return true
}
