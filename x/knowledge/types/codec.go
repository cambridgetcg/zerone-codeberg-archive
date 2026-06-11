package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// RegisterCodec registers the knowledge module's types with the legacy amino codec.
func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgSubmitClaim{}, "zerone_knowledge/SubmitClaim", nil)
	cdc.RegisterConcrete(&MsgSubmitCommitment{}, "zerone_knowledge/SubmitCommitment", nil)
	cdc.RegisterConcrete(&MsgSubmitReveal{}, "zerone_knowledge/SubmitReveal", nil)
	cdc.RegisterConcrete(&MsgChallengeFact{}, "zerone_knowledge/ChallengeFact", nil)
	cdc.RegisterConcrete(&MsgAddFact{}, "zerone_knowledge/AddFact", nil)
	cdc.RegisterConcrete(&MsgSubmitContradiction{}, "zerone_knowledge/SubmitContradiction", nil)
	cdc.RegisterConcrete(&MsgPatronizeFact{}, "zerone_knowledge/PatronizeFact", nil)
	cdc.RegisterConcrete(&MsgProposeDomain{}, "zerone_knowledge/ProposeDomain", nil)
	cdc.RegisterConcrete(&MsgEndorseDomainProposal{}, "zerone_knowledge/EndorseDomainProposal", nil)
	cdc.RegisterConcrete(&MsgChallengeDomainProposal{}, "zerone_knowledge/ChallengeDomainProposal", nil)
	cdc.RegisterConcrete(&MsgRegisterStratum{}, "zerone_knowledge/RegisterStratum", nil)
	cdc.RegisterConcrete(&MsgChallengeProvisionalFact{}, "zerone_knowledge/ChallengeProvisionalFact", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "zerone_knowledge/UpdateParams", nil)
	cdc.RegisterConcrete(&MsgUpdateExtendedParams{}, "zerone_knowledge/UpdateExtendedParams", nil)
	cdc.RegisterConcrete(&MsgProposeResearchFund{}, "zerone_knowledge/ProposeResearchFund", nil)
	cdc.RegisterConcrete(&MsgVoteResearchProposal{}, "zerone_knowledge/VoteResearchProposal", nil)
	cdc.RegisterConcrete(&MsgExecuteResearchProposal{}, "zerone_knowledge/ExecuteResearchProposal", nil)
	cdc.RegisterConcrete(&MsgAddCommonKnowledge{}, "zerone_knowledge/AddCommonKnowledge", nil)
	cdc.RegisterConcrete(&MsgRemoveCommonKnowledge{}, "zerone_knowledge/RemoveCommonKnowledge", nil)
	cdc.RegisterConcrete(&MsgRateFact{}, "zerone_knowledge/RateFact", nil)
	// Route B: training infrastructure
	cdc.RegisterConcrete(&MsgRegisterTrainingPipeline{}, "zerone_knowledge/RegisterTrainingPipeline", nil)
	cdc.RegisterConcrete(&MsgUpdateTrainingPipeline{}, "zerone_knowledge/UpdateTrainingPipeline", nil)
	cdc.RegisterConcrete(&MsgRegisterModelCard{}, "zerone_knowledge/RegisterModelCard", nil)
	cdc.RegisterConcrete(&MsgUpdateModelCard{}, "zerone_knowledge/UpdateModelCard", nil)
	cdc.RegisterConcrete(&MsgRetireModelCard{}, "zerone_knowledge/RetireModelCard", nil)
	// Route B Wave 3
	cdc.RegisterConcrete(&MsgAmendTokenizerSpec{}, "zerone_knowledge/AmendTokenizerSpec", nil)
	cdc.RegisterConcrete(&MsgAttributeContributions{}, "zerone_knowledge/AttributeContributions", nil)
	cdc.RegisterConcrete(&MsgAttestTraining{}, "zerone_knowledge/AttestTraining", nil)
	cdc.RegisterConcrete(&MsgCreateAugmentationBounty{}, "zerone_knowledge/CreateAugmentationBounty", nil)
	cdc.RegisterConcrete(&MsgSubmitAugmentation{}, "zerone_knowledge/SubmitAugmentation", nil)
	cdc.RegisterConcrete(&MsgAcceptAugmentation{}, "zerone_knowledge/AcceptAugmentation", nil)
	// Route B Wave 4
	cdc.RegisterConcrete(&MsgVoteOnAugmentation{}, "zerone_knowledge/VoteOnAugmentation", nil)
	cdc.RegisterConcrete(&MsgSponsorVetoAugmentation{}, "zerone_knowledge/SponsorVetoAugmentation", nil)
	cdc.RegisterConcrete(&MsgChallengeContribution{}, "zerone_knowledge/ChallengeContribution", nil)
	cdc.RegisterConcrete(&MsgResolveContributionChallenge{}, "zerone_knowledge/ResolveContributionChallenge", nil)
	cdc.RegisterConcrete(&MsgClaimTrainingFundDisbursement{}, "zerone_knowledge/ClaimTrainingFundDisbursement", nil)
	// Route B Wave 5
	cdc.RegisterConcrete(&MsgAmendTraceSchema{}, "zerone_knowledge/AmendTraceSchema", nil)
	// Route B Wave 7
	cdc.RegisterConcrete(&MsgCreateTrainingManifest{}, "zerone_knowledge/CreateTrainingManifest", nil)
	cdc.RegisterConcrete(&MsgFinalizeTrainingManifest{}, "zerone_knowledge/FinalizeTrainingManifest", nil)
	cdc.RegisterConcrete(&MsgBindManifestToAttestation{}, "zerone_knowledge/BindManifestToAttestation", nil)
	// Route B Wave 11
	cdc.RegisterConcrete(&MsgOpenIncident{}, "zerone_knowledge/OpenIncident", nil)
	cdc.RegisterConcrete(&MsgRecordRemediation{}, "zerone_knowledge/RecordRemediation", nil)
	cdc.RegisterConcrete(&MsgResolveIncident{}, "zerone_knowledge/ResolveIncident", nil)
	cdc.RegisterConcrete(&MsgCloseIncident{}, "zerone_knowledge/CloseIncident", nil)
	// Route B Wave 12
	cdc.RegisterConcrete(&MsgPauseModule{}, "zerone_knowledge/PauseModule", nil)
	cdc.RegisterConcrete(&MsgUnpauseModule{}, "zerone_knowledge/UnpauseModule", nil)
	// Route B Wave 13
	cdc.RegisterConcrete(&MsgCorrectManifestMerkleRoot{}, "zerone_knowledge/CorrectManifestMerkleRoot", nil)
	// Route B Wave 16
	cdc.RegisterConcrete(&MsgVetoFactInjection{}, "zerone_knowledge/VetoFactInjection", nil)
	// Pragmatism Gap 1
	cdc.RegisterConcrete(&MsgAddConsequenceInstance{}, "zerone_knowledge/AddConsequenceInstance", nil)
	// Scientific Method Gap 2: Methodology Governance
	cdc.RegisterConcrete(&MsgUpdateMethodology{}, "zerone_knowledge/UpdateMethodology", nil)
	// Scientific Method Gap 1: Verifier Independence
	cdc.RegisterConcrete(&MsgSetVerifierOrganization{}, "zerone_knowledge/SetVerifierOrganization", nil)
	// Scientific Method Gap 3: Submission Bias / Pre-registration
	cdc.RegisterConcrete(&MsgPreRegisterClaim{}, "zerone_knowledge/PreRegisterClaim", nil)
	// Contemplative Traditions: Attentional Practice Hours
	cdc.RegisterConcrete(&MsgSetAttentionalPractice{}, "zerone_knowledge/SetAttentionalPractice", nil)
}

// RegisterInterfaces registers the knowledge module's interface implementations.
func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgSubmitClaim{},
		&MsgSubmitCommitment{},
		&MsgSubmitReveal{},
		&MsgChallengeFact{},
		&MsgAddFact{},
		&MsgSubmitContradiction{},
		&MsgPatronizeFact{},
		&MsgProposeDomain{},
		&MsgEndorseDomainProposal{},
		&MsgChallengeDomainProposal{},
		&MsgRegisterStratum{},
		&MsgChallengeProvisionalFact{},
		&MsgUpdateParams{},
		&MsgUpdateExtendedParams{},
		&MsgProposeResearchFund{},
		&MsgVoteResearchProposal{},
		&MsgExecuteResearchProposal{},
		&MsgAddCommonKnowledge{},
		&MsgRemoveCommonKnowledge{},
		&MsgReportDemand{},
		&MsgRateFact{},
		// Route B: training infrastructure
		&MsgRegisterTrainingPipeline{},
		&MsgUpdateTrainingPipeline{},
		&MsgRegisterModelCard{},
		&MsgUpdateModelCard{},
		&MsgRetireModelCard{},
		// Route B Wave 3
		&MsgAmendTokenizerSpec{},
		&MsgAttributeContributions{},
		&MsgAttestTraining{},
		&MsgCreateAugmentationBounty{},
		&MsgSubmitAugmentation{},
		&MsgAcceptAugmentation{},
		// Route B Wave 4
		&MsgVoteOnAugmentation{},
		&MsgSponsorVetoAugmentation{},
		&MsgChallengeContribution{},
		&MsgResolveContributionChallenge{},
		&MsgClaimTrainingFundDisbursement{},
		// Route B Wave 5
		&MsgAmendTraceSchema{},
		// Route B Wave 7
		&MsgCreateTrainingManifest{},
		&MsgFinalizeTrainingManifest{},
		&MsgBindManifestToAttestation{},
		// Route B Wave 11
		&MsgOpenIncident{},
		&MsgRecordRemediation{},
		&MsgResolveIncident{},
		&MsgCloseIncident{},
		// Route B Wave 12
		&MsgPauseModule{},
		&MsgUnpauseModule{},
		// Route B Wave 13
		&MsgCorrectManifestMerkleRoot{},
		// Route B Wave 16
		&MsgVetoFactInjection{},
		// Pragmatism Gap 1
		&MsgAddConsequenceInstance{},
		// Scientific Method Gap 2: Methodology Governance
		&MsgUpdateMethodology{},
		// Scientific Method Gap 1: Verifier Independence
		&MsgSetVerifierOrganization{},
		// Scientific Method Gap 3: Submission Bias / Pre-registration
		&MsgPreRegisterClaim{},
		// Contemplative Traditions: Attentional Practice Hours
		&MsgSetAttentionalPractice{},
	)
}

var (
	Amino     = codec.NewLegacyAmino()
	ModuleCdc = codec.NewProtoCodec(cdctypes.NewInterfaceRegistry())
)
