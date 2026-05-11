// Package app implements the Zerone Cosmos SDK application.
//
// Zerone is a blockchain for AI agent economies using Proof of Truth (PoT)
// consensus, where verifying knowledge IS the useful work.
//
// This file registers all standard Cosmos SDK modules. Custom Zerone modules
// are added incrementally by batch (see REWRITE-PLAN.md).
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/gogoproto/proto"
	gwv2runtime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/gorilla/mux"
	"github.com/spf13/cast"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	"cosmossdk.io/x/evidence"
	evidencekeeper "cosmossdk.io/x/evidence/keeper"
	evidencetypes "cosmossdk.io/x/evidence/types"
	"cosmossdk.io/x/feegrant"
	feegrantkeeper "cosmossdk.io/x/feegrant/keeper"
	feegrantmodule "cosmossdk.io/x/feegrant/module"
	"cosmossdk.io/x/tx/signing"
	"cosmossdk.io/x/upgrade"
	upgradekeeper "cosmossdk.io/x/upgrade/keeper"
	upgradetypes "cosmossdk.io/x/upgrade/types"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	addresscodec "github.com/cosmos/cosmos-sdk/codec/address"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdkruntime "github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/consensus"
	consensuskeeper "github.com/cosmos/cosmos-sdk/x/consensus/keeper"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	// IBC modules
	capability "github.com/cosmos/ibc-go/modules/capability"
	capabilitykeeper "github.com/cosmos/ibc-go/modules/capability/keeper"
	capabilitytypes "github.com/cosmos/ibc-go/modules/capability/types"
	ibctransfer "github.com/cosmos/ibc-go/v8/modules/apps/transfer"
	ibctransferkeeper "github.com/cosmos/ibc-go/v8/modules/apps/transfer/keeper"
	ibctransfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	ibc "github.com/cosmos/ibc-go/v8/modules/core"
	ibcporttypes "github.com/cosmos/ibc-go/v8/modules/core/05-port/types"
	ibcexported "github.com/cosmos/ibc-go/v8/modules/core/exported"
	ibckeeper "github.com/cosmos/ibc-go/v8/modules/core/keeper"

	// IBC Light Clients
	ibctm "github.com/cosmos/ibc-go/v8/modules/light-clients/07-tendermint"
	solomachine "github.com/cosmos/ibc-go/v8/modules/light-clients/06-solomachine"

	// ICA (Interchain Accounts)
	ica "github.com/cosmos/ibc-go/v8/modules/apps/27-interchain-accounts"
	icacontroller "github.com/cosmos/ibc-go/v8/modules/apps/27-interchain-accounts/controller"
	icacontrollerkeeper "github.com/cosmos/ibc-go/v8/modules/apps/27-interchain-accounts/controller/keeper"
	icacontrollertypes "github.com/cosmos/ibc-go/v8/modules/apps/27-interchain-accounts/controller/types"
	icahost "github.com/cosmos/ibc-go/v8/modules/apps/27-interchain-accounts/host"
	icahostkeeper "github.com/cosmos/ibc-go/v8/modules/apps/27-interchain-accounts/host/keeper"
	icahosttypes "github.com/cosmos/ibc-go/v8/modules/apps/27-interchain-accounts/host/types"
	icatypes "github.com/cosmos/ibc-go/v8/modules/apps/27-interchain-accounts/types"

	// IBC Fee Middleware (ICS-29)
	ibcfee "github.com/cosmos/ibc-go/v8/modules/apps/29-fee"
	ibcfeekeeper "github.com/cosmos/ibc-go/v8/modules/apps/29-fee/keeper"
	ibcfeetypes "github.com/cosmos/ibc-go/v8/modules/apps/29-fee/types"

	// CometBFT
	abci "github.com/cometbft/cometbft/abci/types"

	// Crypto codec for keyring support
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"

	// Zerone custom modules
	zeroneauth "github.com/zerone-chain/zerone/x/auth"
	zeroneauthkeeper "github.com/zerone-chain/zerone/x/auth/keeper"
	zeroneauthtypes "github.com/zerone-chain/zerone/x/auth/types"
	zeroneknowledge "github.com/zerone-chain/zerone/x/knowledge"
	zeroneknowledgekeeper "github.com/zerone-chain/zerone/x/knowledge/keeper"
	zeroneknowledgetypes "github.com/zerone-chain/zerone/x/knowledge/types"
	zeroneontology "github.com/zerone-chain/zerone/x/ontology"
	zeroneontologykeeper "github.com/zerone-chain/zerone/x/ontology/keeper"
	zeroneontologytypes "github.com/zerone-chain/zerone/x/ontology/types"
	zeronestaking "github.com/zerone-chain/zerone/x/staking"
	zeronestakingkeeper "github.com/zerone-chain/zerone/x/staking/keeper"
	zeronestakingtypes "github.com/zerone-chain/zerone/x/staking/types"
	zeronebilling "github.com/zerone-chain/zerone/x/billing"
	zeronebillingkeeper "github.com/zerone-chain/zerone/x/billing/keeper"
	zeronebillingtypes "github.com/zerone-chain/zerone/x/billing/types"
	zeroneliquiditypool "github.com/zerone-chain/zerone/x/liquiditypool"
	zeronelpkeeper "github.com/zerone-chain/zerone/x/liquiditypool/keeper"
	zeronelptypes "github.com/zerone-chain/zerone/x/liquiditypool/types"
	zeronetokens "github.com/zerone-chain/zerone/x/tokens"
	zeronetokenskeeper "github.com/zerone-chain/zerone/x/tokens/keeper"
	zeronetokenstypes "github.com/zerone-chain/zerone/x/tokens/types"
	zeronechannels "github.com/zerone-chain/zerone/x/channels"
	zeronechannelskeeper "github.com/zerone-chain/zerone/x/channels/keeper"
	zeronechannelstypes "github.com/zerone-chain/zerone/x/channels/types"
	zeronegov "github.com/zerone-chain/zerone/x/gov"
	zeronegovkeeper "github.com/zerone-chain/zerone/x/gov/keeper"
	zeronegovtypes "github.com/zerone-chain/zerone/x/gov/types"
	zeronehome "github.com/zerone-chain/zerone/x/home"
	zeronehomekeeper "github.com/zerone-chain/zerone/x/home/keeper"
	zeronehometypes "github.com/zerone-chain/zerone/x/home/types"
	zeronepartnerships "github.com/zerone-chain/zerone/x/partnerships"
	zeronepartnershipskeeper "github.com/zerone-chain/zerone/x/partnerships/keeper"
	zeronepartnershipstypes "github.com/zerone-chain/zerone/x/partnerships/types"
	zeroneschedule "github.com/zerone-chain/zerone/x/schedule"
	zeroneschedulekeeper "github.com/zerone-chain/zerone/x/schedule/keeper"
	zeronescheduletypes "github.com/zerone-chain/zerone/x/schedule/types"
	zeronecomputepool "github.com/zerone-chain/zerone/x/compute_pool"
	zeronecpkeeper "github.com/zerone-chain/zerone/x/compute_pool/keeper"
	zeronecptypes "github.com/zerone-chain/zerone/x/compute_pool/types"
	zeronediscovery "github.com/zerone-chain/zerone/x/discovery"
	zeronediscoverykeeper "github.com/zerone-chain/zerone/x/discovery/keeper"
	zeronediscoverytypes "github.com/zerone-chain/zerone/x/discovery/types"
	zeronebvm "github.com/zerone-chain/zerone/x/bvm"
	zeronebvmkeeper "github.com/zerone-chain/zerone/x/bvm/keeper"
	zeronebvmtypes "github.com/zerone-chain/zerone/x/bvm/types"
	vestingrewards "github.com/zerone-chain/zerone/x/vesting_rewards"
	vestingrewardskeeper "github.com/zerone-chain/zerone/x/vesting_rewards/keeper"
	vestingrewardstypes "github.com/zerone-chain/zerone/x/vesting_rewards/types"
	zeronedisputes "github.com/zerone-chain/zerone/x/disputes"
	zeronedisputeskeeper "github.com/zerone-chain/zerone/x/disputes/keeper"
	zeronedisputestypes "github.com/zerone-chain/zerone/x/disputes/types"
	zeronequalification "github.com/zerone-chain/zerone/x/qualification"
	zeronequalificationkeeper "github.com/zerone-chain/zerone/x/qualification/keeper"
	zeronequalificationtypes "github.com/zerone-chain/zerone/x/qualification/types"
	zeroneemergency "github.com/zerone-chain/zerone/x/emergency"
	zeroneemergencykeeper "github.com/zerone-chain/zerone/x/emergency/keeper"
	zeroneemergencytypes "github.com/zerone-chain/zerone/x/emergency/types"
	zeroneibcratelimit "github.com/zerone-chain/zerone/x/ibcratelimit"
	zeroneibcrlkeeper "github.com/zerone-chain/zerone/x/ibcratelimit/keeper"
	zeroneibcrltypes "github.com/zerone-chain/zerone/x/ibcratelimit/types"
	zeroneicaauth "github.com/zerone-chain/zerone/x/icaauth"
	zeroneicaauthkeeper "github.com/zerone-chain/zerone/x/icaauth/keeper"
	zeroneicaauthtypes "github.com/zerone-chain/zerone/x/icaauth/types"
	zeronecapturedefense "github.com/zerone-chain/zerone/x/capture_defense"
	zeronecdkeeper "github.com/zerone-chain/zerone/x/capture_defense/keeper"
	zeronecdtypes "github.com/zerone-chain/zerone/x/capture_defense/types"
	zeronecapturechallenge "github.com/zerone-chain/zerone/x/capture_challenge"
	zeronecckeeper "github.com/zerone-chain/zerone/x/capture_challenge/keeper"
	zeronecctypes "github.com/zerone-chain/zerone/x/capture_challenge/types"
	zeronealignment "github.com/zerone-chain/zerone/x/alignment"
	zeronealignmentkeeper "github.com/zerone-chain/zerone/x/alignment/keeper"
	zeronealignmenttypes "github.com/zerone-chain/zerone/x/alignment/types"
	zeroneresearch "github.com/zerone-chain/zerone/x/research"
	zeroneresearchkeeper "github.com/zerone-chain/zerone/x/research/keeper"
	zeroneresearchtypes "github.com/zerone-chain/zerone/x/research/types"
	zeroneprovenance "github.com/zerone-chain/zerone/x/training_provenance"
	zeroneprovenancekeeper "github.com/zerone-chain/zerone/x/training_provenance/keeper"
	zeroneprovenancetypes "github.com/zerone-chain/zerone/x/training_provenance/types"
	zeronetrustscore "github.com/zerone-chain/zerone/x/trust_score"
	zeronetrustscorekeeper "github.com/zerone-chain/zerone/x/trust_score/keeper"
	zeronetrustscoretypes "github.com/zerone-chain/zerone/x/trust_score/types"
	zeronegovsynth "github.com/zerone-chain/zerone/x/governance_synthesis"
	zeronegovsynthkeeper "github.com/zerone-chain/zerone/x/governance_synthesis/keeper"
	zeronegovsynthtypes "github.com/zerone-chain/zerone/x/governance_synthesis/types"
	zeroneprivatecorpus "github.com/zerone-chain/zerone/x/private_corpus"
	zeroneprivatecorpuskeeper "github.com/zerone-chain/zerone/x/private_corpus/keeper"
	zeroneprivatecorpustypes "github.com/zerone-chain/zerone/x/private_corpus/types"
	zeronecounterex "github.com/zerone-chain/zerone/x/counterexamples"
	zeronecounterexkeeper "github.com/zerone-chain/zerone/x/counterexamples/keeper"
	zeronecounterextypes "github.com/zerone-chain/zerone/x/counterexamples/types"
	zeroneinquiry "github.com/zerone-chain/zerone/x/inquiry"
	zeroneinquirykeeper "github.com/zerone-chain/zerone/x/inquiry/keeper"
	zeroneinquirytypes "github.com/zerone-chain/zerone/x/inquiry/types"
	zeroneagentu "github.com/zerone-chain/zerone/x/agent_understanding"
	zeroneagentukeeper "github.com/zerone-chain/zerone/x/agent_understanding/keeper"
	zeroneagentutypes "github.com/zerone-chain/zerone/x/agent_understanding/types"
	zeronedialectic "github.com/zerone-chain/zerone/x/dialectic"
	zeronedialectickeeper "github.com/zerone-chain/zerone/x/dialectic/keeper"
	zeronedialectictypes "github.com/zerone-chain/zerone/x/dialectic/types"
	zeroneautopoiesis "github.com/zerone-chain/zerone/x/autopoiesis"
	zeroneapkeeper "github.com/zerone-chain/zerone/x/autopoiesis/keeper"
	zeroneaptypes "github.com/zerone-chain/zerone/x/autopoiesis/types"
	zeroneevidencemgmt "github.com/zerone-chain/zerone/x/evidence_mgmt"
	zeroneemkeeper "github.com/zerone-chain/zerone/x/evidence_mgmt/keeper"
	zeroneemtypes "github.com/zerone-chain/zerone/x/evidence_mgmt/types"
	zeroneclaimingpot "github.com/zerone-chain/zerone/x/claiming_pot"
	zeronecpotkeeper "github.com/zerone-chain/zerone/x/claiming_pot/keeper"
	zeronecpottypes "github.com/zerone-chain/zerone/x/claiming_pot/types"
	zeronetoolbox "github.com/zerone-chain/zerone/x/toolbox"
	zeronetoolboxkeeper "github.com/zerone-chain/zerone/x/toolbox/keeper"
	zeronetoolboxtypes "github.com/zerone-chain/zerone/x/toolbox/types"
	zeronetree "github.com/zerone-chain/zerone/x/tree"
	zeronetreekeeper "github.com/zerone-chain/zerone/x/tree/keeper"
	zeronettreetypes "github.com/zerone-chain/zerone/x/tree/types"

	// Swagger UI (embedded)
	swagger "github.com/zerone-chain/zerone/docs/swagger-ui"

	// Tx types (cosmos.tx.v1beta1.Tx registration)
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"

	// gRPC services
	cmtservice "github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	nodeservice "github.com/cosmos/cosmos-sdk/client/grpc/node"
)

// App-level constants are defined in app/constants.go.

var (
	// DefaultNodeHome is the default home directory for the node.
	DefaultNodeHome string

	// ModuleBasics defines the module BasicManager used for codec registration
	// and genesis verification.
	ModuleBasics = module.NewBasicManager(
		auth.AppModuleBasic{},
		genutil.NewAppModuleBasic(genutiltypes.DefaultMessageValidator),
		bank.AppModuleBasic{},
		staking.AppModuleBasic{},
		distr.AppModuleBasic{},
		gov.NewAppModuleBasic(nil),
		slashing.AppModuleBasic{},
		feegrantmodule.AppModuleBasic{},
		upgrade.AppModuleBasic{},
		evidence.AppModuleBasic{},
		vesting.AppModuleBasic{},
		consensus.AppModuleBasic{},
		ibc.AppModuleBasic{},
		ibctransfer.AppModuleBasic{},
		ibcfee.AppModuleBasic{},
		ica.AppModuleBasic{},
		// ===== Zerone custom modules — added by batch =====
		zeroneauth.AppModuleBasic{},
		zeronestaking.AppModuleBasic{},
		vestingrewards.AppModuleBasic{},
		zeroneontology.AppModuleBasic{},
		zeroneknowledge.AppModuleBasic{},
		zeronetokens.AppModuleBasic{},
		zeronebilling.AppModuleBasic{},
		zeroneliquiditypool.AppModuleBasic{},
		zeronegov.AppModuleBasic{},
		zeronechannels.AppModuleBasic{},
		zeronehome.AppModuleBasic{},
		zeroneschedule.AppModuleBasic{},
		zeronecomputepool.AppModuleBasic{},
		zeronediscovery.AppModuleBasic{},
		zeronebvm.AppModuleBasic{},
		zeronedisputes.AppModuleBasic{},
		zeronequalification.AppModuleBasic{},
		zeroneemergency.AppModuleBasic{},
		// R2-2: x/knowledge wired
		// R3-1: x/billing — wired
		// R3-2: x/liquiditypool — wired
		// R3-4: x/gov — wired
		// R3-6: x/tokens — wired
		// R4-1: x/home
		// R4-2: x/partnerships
		// R4-3: x/bvm
		// R4-4: x/channels
		// R4-5: x/schedule, x/compute_pool, x/discovery
		// R5-1: x/toolbox
		// R6-1: x/emergency
		// R6-2: x/evidence_mgmt
		// R6-3: x/disputes
		// R6-4: x/capture_challenge, x/capture_defense
		// R6-5: x/qualification
		// R6-6: x/ibcratelimit, x/icaauth — wired
		zeroneibcratelimit.AppModuleBasic{},
		zeroneicaauth.AppModuleBasic{},
		zeronecapturedefense.AppModuleBasic{},
		zeronecapturechallenge.AppModuleBasic{},
		zeroneresearch.AppModuleBasic{},
		zeronealignment.AppModuleBasic{},
		zeroneautopoiesis.AppModuleBasic{}, // R7-1: x/autopoiesis
		zeroneevidencemgmt.AppModuleBasic{},
		zeroneclaimingpot.AppModuleBasic{},
		zeronetree.AppModuleBasic{},         // R7-5: x/tree
		zeronepartnerships.AppModuleBasic{}, // R8-1: x/partnerships
		zeronetoolbox.AppModuleBasic{},      // R8-1: x/toolbox
		zeroneprivatecorpus.AppModuleBasic{}, // x/private_corpus: off-chain vault references
		zeronecounterex.AppModuleBasic{},     // x/counterexamples: alignment-by-structure
		zeroneinquiry.AppModuleBasic{},       // x/inquiry: open-question market for unmapped territory
		zeroneagentu.AppModuleBasic{},        // x/agent_understanding: per-agent topic profile synthesizer
		zeronedialectic.AppModuleBasic{},     // x/dialectic: disagreement-shape synthesizer (commitment 17)
	)

	// Module account permissions.
	maccPerms = map[string][]string{
		authtypes.FeeCollectorName:     nil,
		distrtypes.ModuleName:          nil,
		stakingtypes.BondedPoolName:    {authtypes.Burner, authtypes.Staking},
		stakingtypes.NotBondedPoolName: {authtypes.Burner, authtypes.Staking},
		govtypes.ModuleName:            {authtypes.Burner},
		ibctransfertypes.ModuleName:    {authtypes.Minter, authtypes.Burner},
		ibcfeetypes.ModuleName:         nil,
		icatypes.ModuleName:            nil,
		// ===== Zerone custom module accounts — added by batch =====
		zeroneauthtypes.ModuleName:    {authtypes.Minter}, // Minter for bootstrap fund
		zeronestakingtypes.ModuleName: {authtypes.Burner, authtypes.Staking},
		vestingrewardstypes.ModuleName:        {authtypes.Minter, authtypes.Burner}, // Minter for block rewards, Burner retained for interface compat
		vestingrewardstypes.ResearchFundModuleName:    nil,                           // research_fund: receive-only
		vestingrewardstypes.DevelopmentFundModuleName: nil,                           // development_fund: receive-only
		zeroneontologytypes.ModuleName:             nil,                              // ontology: receive proposal stake
		zeroneknowledgetypes.ModuleName:            {authtypes.Burner},               // knowledge: burn slashed claim stakes
		zeroneknowledgetypes.BootstrapFundModuleName:    {authtypes.Minter},              // knowledge_bootstrap_fund: genesis mint
		zeroneknowledgetypes.TrainingFundModuleName:      {authtypes.Minter},              // knowledge_training_fund: Wave 4 augmentation escrow + post-hoc disbursements + vesting
		zeroneknowledgetypes.ProbeBountyPoolModuleName:   {authtypes.Minter},              // knowledge_probe_bounty_pool: Wave 15 per-block-minted probe rewards
		zeroneknowledgetypes.VindicationEscrowModuleName: nil,                           // vindication_escrow: holds minority slashes until vindication or expiry
		zeroneinquirytypes.BountyPoolModuleName:          nil,                           // inquiry_bounty_pool: receive-only escrow for inquiry bounties
		zeronetokenstypes.ModuleName:               {authtypes.Minter, authtypes.Burner}, // tokens: mint/burn for wrap/unwrap + emissions
		zeronebillingtypes.ModuleName:              {authtypes.Burner},                        // billing: revenue split
		zeronelptypes.ModuleName:                   {authtypes.Minter, authtypes.Burner}, // liquiditypool: mint/burn LP tokens
		zeronegovtypes.ModuleName:                  nil,                                  // gov: receive stake deposits
		zeronechannelstypes.ModuleName:             nil,                                  // channels: escrow deposits
		zeronehometypes.ModuleName:                 nil,                                  // home: no mint/burn
		zeronescheduletypes.ModuleName:             nil,                                  // schedule: fee escrow
		zeronecptypes.ModuleName:                   {authtypes.Burner, authtypes.Staking}, // compute_pool: stake escrow
		zeronediscoverytypes.ModuleName:            nil,                                  // discovery: stake escrow
		"protocol_treasury":                        nil,                                  // protocol_treasury: receive revenue split
		zeronebvmtypes.ModuleName:                  {authtypes.Burner},                   // bvm: burn deploy fees
		zeronedisputestypes.ModuleName:             {authtypes.Burner},                   // disputes: bond escrow + burn
		zeronequalificationtypes.ModuleName:        nil,                                  // qualification: stake escrow
		zeroneemergencytypes.ModuleName:            nil,                                  // emergency: no mint/burn — signal-only module
		zeronecctypes.ModuleName:                   {authtypes.Burner},                   // capture_challenge: rejected stakes to dev fund
		zeronecdtypes.ModuleName:                   nil,                                  // capture_defense: no mint/burn
		zeroneibcrltypes.ModuleName:                nil,                                  // ibcratelimit: no mint/burn — middleware only
		zeroneicaauthtypes.ModuleName:              nil,                                  // icaauth: no mint/burn — auth wrapper only
		zeroneresearchtypes.ModuleName:             {authtypes.Burner},                   // research: stake slashing burns
		zeronealignmenttypes.ModuleName:            nil,                                  // alignment: no mint/burn — signal-only module
		zeroneaptypes.ModuleName:                   nil,                                  // autopoiesis: no mint/burn — signal-only module
		zeroneemtypes.ModuleName:                   {authtypes.Burner},                   // evidence_mgmt: burn challenged bonds
		zeronecpottypes.ModuleName:                 nil,                                  // claiming_pot: receive-only, bank sends from module
		zeronettreetypes.ModuleName:                {authtypes.Burner},                   // tree: revenue split
		zeronepartnershipstypes.ModuleName:         {authtypes.Burner},                   // partnerships: dissolved stakes to dev fund
		zeronetoolboxtypes.ModuleName:              {authtypes.Burner},                   // toolbox: deregistration fees
		"treasury_protocol":                        nil,                                  // treasury_protocol: receive-only
	}
)

func init() {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	DefaultNodeHome = filepath.Join(userHomeDir, ".zeroned")

	// Set bech32 prefixes for Zerone addresses.
	sdkConfig := sdk.GetConfig()
	sdkConfig.SetBech32PrefixForAccount(AccountAddressPrefix, AccountAddressPrefix+"pub")
	sdkConfig.SetBech32PrefixForValidator(AccountAddressPrefix+"valoper", AccountAddressPrefix+"valoperpub")
	sdkConfig.SetBech32PrefixForConsensusNode(AccountAddressPrefix+"valcons", AccountAddressPrefix+"valconspub")
	sdkConfig.Seal()

	// Set the default bond denom to uzrn (micro-ZRN).
	sdk.DefaultBondDenom = BondDenom
}

// EncodingConfig specifies the concrete encoding types to use for a given app.
type EncodingConfig struct {
	InterfaceRegistry codectypes.InterfaceRegistry
	Codec             codec.Codec
	TxConfig          client.TxConfig
	Amino             *codec.LegacyAmino
}

// MakeEncodingConfig creates the EncodingConfig for the Zerone application.
func MakeEncodingConfig() EncodingConfig {
	interfaceRegistry, err := codectypes.NewInterfaceRegistryWithOptions(codectypes.InterfaceRegistryOptions{
		ProtoFiles: proto.HybridResolver,
		SigningOptions: signing.Options{
			AddressCodec:          addresscodec.NewBech32Codec(AccountAddressPrefix),
			ValidatorAddressCodec: addresscodec.NewBech32Codec(AccountAddressPrefix + "valoper"),
		},
	})
	if err != nil {
		panic(err)
	}

	appCodec := codec.NewProtoCodec(interfaceRegistry)
	legacyAmino := codec.NewLegacyAmino()
	txConfig := authtx.NewTxConfig(appCodec, authtx.DefaultSignModes)

	sdk.RegisterLegacyAminoCodec(legacyAmino)
	sdk.RegisterInterfaces(interfaceRegistry)
	cryptocodec.RegisterInterfaces(interfaceRegistry)
	ModuleBasics.RegisterInterfaces(interfaceRegistry)
	ModuleBasics.RegisterLegacyAminoCodec(legacyAmino)
	txtypes.RegisterInterfaces(interfaceRegistry)

	return EncodingConfig{
		InterfaceRegistry: interfaceRegistry,
		Codec:             appCodec,
		TxConfig:          txConfig,
		Amino:             legacyAmino,
	}
}

// GenesisState is the top-level genesis state: module name → raw genesis bytes.
type GenesisState map[string]json.RawMessage

// ZeroneApp extends baseapp.BaseApp with all Cosmos SDK modules.
type ZeroneApp struct {
	*baseapp.BaseApp

	legacyAmino       *codec.LegacyAmino
	appCodec          codec.Codec
	txConfig          client.TxConfig
	interfaceRegistry codectypes.InterfaceRegistry

	// Store keys
	keys    map[string]*storetypes.KVStoreKey
	tkeys   map[string]*storetypes.TransientStoreKey
	memKeys map[string]*storetypes.MemoryStoreKey

	// ---------- Standard Cosmos SDK Keepers ----------
	AccountKeeper   authkeeper.AccountKeeper
	BankKeeper      bankkeeper.Keeper
	StakingKeeper   *stakingkeeper.Keeper
	SlashingKeeper  slashingkeeper.Keeper
	DistrKeeper     distrkeeper.Keeper
	GovKeeper       *govkeeper.Keeper
	UpgradeKeeper   *upgradekeeper.Keeper
	EvidenceKeeper  evidencekeeper.Keeper
	FeeGrantKeeper  feegrantkeeper.Keeper
	ConsensusKeeper consensuskeeper.Keeper

	// ---------- IBC Keepers ----------
	CapabilityKeeper          *capabilitykeeper.Keeper
	ScopedIBCKeeper           capabilitykeeper.ScopedKeeper
	ScopedTransferKeeper      capabilitykeeper.ScopedKeeper
	ScopedICAControllerKeeper capabilitykeeper.ScopedKeeper
	ScopedICAHostKeeper       capabilitykeeper.ScopedKeeper
	IBCKeeper                 *ibckeeper.Keeper
	IBCFeeKeeper              ibcfeekeeper.Keeper
	TransferKeeper            ibctransferkeeper.Keeper
	ICAControllerKeeper       icacontrollerkeeper.Keeper
	ICAHostKeeper             icahostkeeper.Keeper

	// ===== Zerone custom module keepers — added by batch =====
	ZeroneAuthKeeper        zeroneauthkeeper.Keeper
	ZeroneStakingKeeper     zeronestakingkeeper.Keeper
	VestingRewardsKeeper    vestingrewardskeeper.Keeper
	ZeroneOntologyKeeper    zeroneontologykeeper.Keeper
	KnowledgeKeeper         zeroneknowledgekeeper.Keeper
	TokensKeeper            zeronetokenskeeper.Keeper
	BillingKeeper           zeronebillingkeeper.Keeper
	LiquidityPoolKeeper     zeronelpkeeper.Keeper
	ZeroneGovKeeper         zeronegovkeeper.Keeper
	ChannelsKeeper          zeronechannelskeeper.Keeper
	ScheduleKeeper          zeroneschedulekeeper.Keeper
	ComputePoolKeeper       zeronecpkeeper.Keeper
	DiscoveryKeeper         zeronediscoverykeeper.Keeper
	BVMKeeper               zeronebvmkeeper.Keeper
	DisputesKeeper          zeronedisputeskeeper.Keeper
	QualificationKeeper     zeronequalificationkeeper.Keeper
	EmergencyKeeper         zeroneemergencykeeper.Keeper
	CaptureDefenseKeeper    zeronecdkeeper.Keeper
	CaptureChallengeKeeper  zeronecckeeper.Keeper
	HomeKeeper              zeronehomekeeper.Keeper
	PartnershipsKeeper      zeronepartnershipskeeper.Keeper
	ToolboxKeeper           zeronetoolboxkeeper.Keeper
	IBCRateLimitKeeper  zeroneibcrlkeeper.Keeper
	ICAAuthKeeper       zeroneicaauthkeeper.Keeper
	ResearchKeeper          zeroneresearchkeeper.Keeper
	TrainingProvenanceKeeper zeroneprovenancekeeper.Keeper
	TrustScoreKeeper         zeronetrustscorekeeper.Keeper
	GovernanceSynthesisKeeper zeronegovsynthkeeper.Keeper
	AlignmentKeeper         zeronealignmentkeeper.Keeper
	AutopoiesisKeeper       zeroneapkeeper.Keeper // R7-1: autopoiesis
	EvidenceMgmtKeeper      zeroneemkeeper.Keeper
	ClaimingPotKeeper       zeronecpotkeeper.Keeper
	TreeKeeper              zeronetreekeeper.Keeper // R7-5: x/tree
	PrivateCorpusKeeper     zeroneprivatecorpuskeeper.Keeper // x/private_corpus: off-chain vault references
	CounterexamplesKeeper   zeronecounterexkeeper.Keeper     // x/counterexamples: alignment-by-structure (commitment 15)
	InquiryKeeper           zeroneinquirykeeper.Keeper       // x/inquiry: open-question market (commitment 16)
	AgentUnderstandingKeeper zeroneagentukeeper.Keeper       // x/agent_understanding: per-agent profile synthesizer (extends commitment 11)
	DialecticKeeper          zeronedialectickeeper.Keeper    // x/dialectic: disagreement-shape synthesizer (commitment 17)

	// ABCI++ vote extension config (nil until validator is configured)
	VoteExtConfig *VoteExtensionConfig

	// Oracle client for querying the evaluation sidecar (nil if disabled).
	// Stored here so it can be attached to VoteExtConfig when the validator is configured.
	oracleClient OracleClient

	// Module manager
	ModuleManager *module.Manager

	// Simulation manager (for fuzz testing)
	sm *module.SimulationManager

	// Configurator for module msg/query registration
	configurator module.Configurator
}

// SetVoteExtConfig configures the validator's vote extension settings.
// If an oracle client was initialized from app.toml, it is automatically
// attached to the config so the vote extension handler can query it.
func (app *ZeroneApp) SetVoteExtConfig(config *VoteExtensionConfig) {
	if app.oracleClient != nil && config != nil {
		config.OracleClient = app.oracleClient
	}
	app.VoteExtConfig = config
}

// NewZeroneApp creates and initializes a new ZeroneApp instance.
func NewZeroneApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	loadLatest bool,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) *ZeroneApp {
	interfaceRegistry, err := codectypes.NewInterfaceRegistryWithOptions(codectypes.InterfaceRegistryOptions{
		ProtoFiles: proto.HybridResolver,
		SigningOptions: signing.Options{
			AddressCodec:          addresscodec.NewBech32Codec(AccountAddressPrefix),
			ValidatorAddressCodec: addresscodec.NewBech32Codec(AccountAddressPrefix + "valoper"),
		},
	})
	if err != nil {
		panic(err)
	}
	appCodec := codec.NewProtoCodec(interfaceRegistry)
	legacyAmino := codec.NewLegacyAmino()
	txConfig := authtx.NewTxConfig(appCodec, authtx.DefaultSignModes)

	sdk.RegisterLegacyAminoCodec(legacyAmino)
	sdk.RegisterInterfaces(interfaceRegistry)
	cryptocodec.RegisterInterfaces(interfaceRegistry)
	ModuleBasics.RegisterInterfaces(interfaceRegistry)
	ModuleBasics.RegisterLegacyAminoCodec(legacyAmino)
	txtypes.RegisterInterfaces(interfaceRegistry)
	// IBC light client types must be registered for tx decoding (Any unpacking).
	// Registered here rather than in ModuleBasics because their DefaultGenesis returns nil.
	ibctm.RegisterInterfaces(interfaceRegistry)
	solomachine.RegisterInterfaces(interfaceRegistry)

	bApp := baseapp.NewBaseApp(AppName, logger, db, txConfig.TxDecoder(), baseAppOptions...)
	bApp.SetCommitMultiStoreTracer(traceStore)
	bApp.SetVersion(version.Version)
	bApp.SetInterfaceRegistry(interfaceRegistry)

	// ---- Store Keys ----
	keys := storetypes.NewKVStoreKeys(
		authtypes.StoreKey,
		banktypes.StoreKey,
		stakingtypes.StoreKey,
		distrtypes.StoreKey,
		slashingtypes.StoreKey,
		govtypes.StoreKey,
		upgradetypes.StoreKey,
		feegrant.StoreKey,
		evidencetypes.StoreKey,
		capabilitytypes.StoreKey,
		ibcexported.StoreKey,
		ibctransfertypes.StoreKey,
		ibcfeetypes.StoreKey,
		icacontrollertypes.StoreKey,
		icahosttypes.StoreKey,
		"consensus", // x/consensus module store key
		// ===== Zerone custom module store keys — added by batch =====
		zeroneauthtypes.StoreKey,
		zeronestakingtypes.StoreKey,
		vestingrewardstypes.StoreKey,
		zeroneontologytypes.StoreKey,
		zeroneknowledgetypes.StoreKey,
		zeronetokenstypes.StoreKey,
		zeronebillingtypes.StoreKey,
		zeronelptypes.StoreKey,
		zeronegovtypes.StoreKey,
		zeronehometypes.StoreKey,
		zeronepartnershipstypes.StoreKey,
		zeronetoolboxtypes.StoreKey,
		zeronechannelstypes.StoreKey,
		zeronescheduletypes.StoreKey,
		zeronecptypes.StoreKey,
		zeronediscoverytypes.StoreKey,
		zeronebvmtypes.StoreKey,
		zeronedisputestypes.StoreKey,
		zeronequalificationtypes.StoreKey,
		zeroneemergencytypes.StoreKey,
		zeronecdtypes.StoreKey,
		zeronecctypes.StoreKey,
		zeroneibcrltypes.StoreKey,
		zeroneicaauthtypes.StoreKey,
		zeroneresearchtypes.StoreKey,
		zeronealignmenttypes.StoreKey,
		zeroneaptypes.StoreKey,
		zeroneemtypes.StoreKey,
		zeronecpottypes.StoreKey,
		zeronettreetypes.StoreKey,
		zeroneprivatecorpustypes.StoreKey,
		zeronecounterextypes.StoreKey,
		zeroneinquirytypes.StoreKey,
		zeroneagentutypes.StoreKey,
		zeronedialectictypes.StoreKey,
	)
	tkeys := storetypes.NewTransientStoreKeys(paramstypes.TStoreKey)
	memKeys := storetypes.NewMemoryStoreKeys(capabilitytypes.MemStoreKey)

	app := &ZeroneApp{
		BaseApp:           bApp,
		legacyAmino:       legacyAmino,
		appCodec:          appCodec,
		txConfig:          txConfig,
		interfaceRegistry: interfaceRegistry,
		keys:              keys,
		tkeys:             tkeys,
		memKeys:           memKeys,
	}

	// ---- Module Keepers ----

	app.ConsensusKeeper = consensuskeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys["consensus"]),
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		sdkruntime.EventService{},
	)
	bApp.SetParamStore(app.ConsensusKeeper.ParamsStore)

	app.AccountKeeper = authkeeper.NewAccountKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[authtypes.StoreKey]),
		authtypes.ProtoBaseAccount,
		maccPerms,
		addresscodec.NewBech32Codec(AccountAddressPrefix),
		AccountAddressPrefix,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.BankKeeper = bankkeeper.NewBaseKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[banktypes.StoreKey]),
		app.AccountKeeper,
		blockedModuleAccountAddrs(),
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		logger,
	)

	app.StakingKeeper = stakingkeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[stakingtypes.StoreKey]),
		app.AccountKeeper,
		app.BankKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		addresscodec.NewBech32Codec(AccountAddressPrefix+"valoper"),
		addresscodec.NewBech32Codec(AccountAddressPrefix+"valcons"),
	)

	app.DistrKeeper = distrkeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[distrtypes.StoreKey]),
		app.AccountKeeper,
		app.BankKeeper,
		app.StakingKeeper,
		authtypes.FeeCollectorName,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.SlashingKeeper = slashingkeeper.NewKeeper(
		appCodec,
		legacyAmino,
		sdkruntime.NewKVStoreService(keys[slashingtypes.StoreKey]),
		app.StakingKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.FeeGrantKeeper = feegrantkeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[feegrant.StoreKey]),
		app.AccountKeeper,
	)

	app.UpgradeKeeper = upgradekeeper.NewKeeper(
		skipUpgradeHeights(appOpts),
		sdkruntime.NewKVStoreService(keys[upgradetypes.StoreKey]),
		appCodec,
		DefaultNodeHome,
		app.BaseApp,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.EvidenceKeeper = *evidencekeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[evidencetypes.StoreKey]),
		app.StakingKeeper,
		app.SlashingKeeper,
		app.AccountKeeper.AddressCodec(),
		sdkruntime.ProvideCometInfoService(),
	)

	// ---- Staking Hooks ----
	// Wire slashing and distribution as hooks on staking so that validator
	// signing info is created when validators are added during genesis.
	app.StakingKeeper.SetHooks(
		stakingtypes.NewMultiStakingHooks(app.DistrKeeper.Hooks(), app.SlashingKeeper.Hooks()),
	)

	// ---- Governance Keeper ----
	govConfig := govtypes.DefaultConfig()
	app.GovKeeper = govkeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[govtypes.StoreKey]),
		app.AccountKeeper,
		app.BankKeeper,
		app.StakingKeeper,
		app.DistrKeeper,
		app.MsgServiceRouter(),
		govConfig,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	// ---- Capability Keeper (required by IBC) ----
	app.CapabilityKeeper = capabilitykeeper.NewKeeper(
		appCodec,
		keys[capabilitytypes.StoreKey],
		memKeys[capabilitytypes.MemStoreKey],
	)
	app.ScopedIBCKeeper = app.CapabilityKeeper.ScopeToModule(ibcexported.ModuleName)
	app.ScopedTransferKeeper = app.CapabilityKeeper.ScopeToModule(ibctransfertypes.ModuleName)
	app.ScopedICAControllerKeeper = app.CapabilityKeeper.ScopeToModule(icacontrollertypes.SubModuleName)
	app.ScopedICAHostKeeper = app.CapabilityKeeper.ScopeToModule(icahosttypes.SubModuleName)
	// Seal after all ScopeToModule calls — prevents capability escalation at runtime.
	app.CapabilityKeeper.Seal()

	// ---- IBC Keepers ----
	app.IBCKeeper = ibckeeper.NewKeeper(
		appCodec,
		keys[ibcexported.StoreKey],
		paramstypes.Subspace{}, // x/params removed in v0.47+; IBC accepts empty subspace
		app.StakingKeeper,
		app.UpgradeKeeper,
		app.ScopedIBCKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.IBCFeeKeeper = ibcfeekeeper.NewKeeper(
		appCodec,
		keys[ibcfeetypes.StoreKey],
		app.IBCKeeper.ChannelKeeper, // ics4Wrapper
		app.IBCKeeper.ChannelKeeper,
		app.IBCKeeper.PortKeeper,
		app.AccountKeeper,
		app.BankKeeper,
	)

	// IBCRateLimitKeeper must be created before TransferKeeper so it can intercept outbound SendPacket.
	app.IBCRateLimitKeeper = zeroneibcrlkeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeroneibcrltypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	// SECURITY: Rate limit ICS4Wrapper intercepts outbound SendPacket for quota enforcement.
	// Created before TransferKeeper so it can be injected as the ICS4Wrapper in the outbound chain.
	rateLimitICS4 := zeroneibcratelimit.NewIBCMiddleware(
		nil,              // IBCModule set later (only ICS4Wrapper used here)
		app.IBCFeeKeeper, // inner ICS4Wrapper for SendPacket forwarding
		app.IBCRateLimitKeeper,
	)

	app.TransferKeeper = ibctransferkeeper.NewKeeper(
		appCodec,
		keys[ibctransfertypes.StoreKey],
		paramstypes.Subspace{},
		rateLimitICS4,                // ics4Wrapper routes through rate limit then fee middleware
		app.IBCKeeper.ChannelKeeper,
		app.IBCKeeper.PortKeeper,
		app.AccountKeeper,
		app.BankKeeper,
		app.ScopedTransferKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.ICAControllerKeeper = icacontrollerkeeper.NewKeeper(
		appCodec,
		keys[icacontrollertypes.StoreKey],
		paramstypes.Subspace{},
		app.IBCKeeper.ChannelKeeper, // ics4Wrapper
		app.IBCKeeper.ChannelKeeper,
		app.IBCKeeper.PortKeeper,
		app.ScopedICAControllerKeeper,
		app.MsgServiceRouter(),
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.ICAHostKeeper = icahostkeeper.NewKeeper(
		appCodec,
		keys[icahosttypes.StoreKey],
		paramstypes.Subspace{},
		app.IBCKeeper.ChannelKeeper,
		app.IBCKeeper.ChannelKeeper,
		app.IBCKeeper.PortKeeper,
		app.AccountKeeper,
		app.ScopedICAHostKeeper,
		app.MsgServiceRouter(),
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)
	app.ICAHostKeeper.WithQueryRouter(app.GRPCQueryRouter())

	// ===== Zerone custom module keeper init (added by batch) =====

	app.ZeroneAuthKeeper = zeroneauthkeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[zeroneauthtypes.StoreKey]),
		app.AccountKeeper,
		app.BankKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.ZeroneStakingKeeper = zeronestakingkeeper.NewKeeper(
		appCodec,
		keys[zeronestakingtypes.StoreKey],
		app.AccountKeeper,
		app.BankKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.VestingRewardsKeeper = vestingrewardskeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[vestingrewardstypes.StoreKey]),
		app.BankKeeper,
		nil, // staking keeper set after x/staking wiring
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.ZeroneOntologyKeeper = zeroneontologykeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[zeroneontologytypes.StoreKey]),
		app.BankKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	stakingAdapter := zeronestakingkeeper.NewStakingKeeperAdapter(app.ZeroneStakingKeeper)
	app.KnowledgeKeeper = zeroneknowledgekeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeroneknowledgetypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
		stakingAdapter,
	)
	app.KnowledgeKeeper.SetOntologyKeeper(&app.ZeroneOntologyKeeper)
	app.KnowledgeKeeper.SetVestingRewardsKeeper(vestingrewardskeeper.NewVestingRewardsKeeperAdapter(app.VestingRewardsKeeper))

	app.TokensKeeper = zeronetokenskeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[zeronetokenstypes.StoreKey]),
		app.BankKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	billingKnowledgeAdapter := zeroneknowledgekeeper.NewBillingKnowledgeAdapter(app.KnowledgeKeeper)
	vestingRFDAdapter := vestingrewardskeeper.NewResearchFundDepositorAdapter(app.VestingRewardsKeeper)
	billingBankAdapter := zeronebillingkeeper.NewBillingBankAdapter(app.BankKeeper)
	app.BillingKeeper = zeronebillingkeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronebillingtypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		billingBankAdapter,
		billingKnowledgeAdapter,
		vestingRFDAdapter,
	)

	app.LiquidityPoolKeeper = zeronelpkeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[zeronelptypes.StoreKey]),
		app.BankKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	// Wire liquidity pool TWAP oracle into billing for dynamic pricing.
	app.BillingKeeper.SetLiquidityPoolKeeper(
		zeronelpkeeper.NewLiquidityPoolKeeperAdapter(app.LiquidityPoolKeeper),
	)

	govStakingAdapter := zeronestakingkeeper.NewGovStakingKeeperAdapter(app.ZeroneStakingKeeper)
	app.ZeroneGovKeeper = zeronegovkeeper.NewKeeper(
		appCodec,
		keys[zeronegovtypes.StoreKey],
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
		govStakingAdapter,
	)
	app.ZeroneGovKeeper.SetVestingKeeper(&app.VestingRewardsKeeper)
	app.ZeroneGovKeeper.SetUpgradeKeeper(NewGovUpgradeAdapter(app.UpgradeKeeper))
	app.ZeroneGovKeeper.SetParamRouter(NewGovParamRouter())

	app.ChannelsKeeper = zeronechannelskeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronechannelstypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
	)

	app.ScheduleKeeper = zeroneschedulekeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronescheduletypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
	)

	app.ComputePoolKeeper = zeronecpkeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronecptypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
	)

	app.DiscoveryKeeper = zeronediscoverykeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronediscoverytypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
	)

	app.BVMKeeper = zeronebvmkeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[zeronebvmtypes.StoreKey]),
		app.BankKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)
	bvmKnowledgeAdapter := zeroneknowledgekeeper.NewBVMKnowledgeAdapter(app.KnowledgeKeeper)
	app.BVMKeeper.SetKnowledgeKeeper(bvmKnowledgeAdapter)
	bvmAuthAdapter := zeroneauthkeeper.NewBVMAuthAdapter(app.ZeroneAuthKeeper)
	app.BVMKeeper.SetAuthKeeper(bvmAuthAdapter)
	app.BVMKeeper.SetHomeKeeper(zeronehomekeeper.NewBVMHomeAdapter(app.HomeKeeper))

	disputesStakingAdapter := zeronestakingkeeper.NewDisputesStakingKeeperAdapter(app.ZeroneStakingKeeper)
	disputesKnowledgeAdapter := zeroneknowledgekeeper.NewDisputesKnowledgeAdapter(app.KnowledgeKeeper)
	app.DisputesKeeper = zeronedisputeskeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronedisputestypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
		disputesStakingAdapter,
		disputesKnowledgeAdapter,
	)

	qualificationStakingAdapter := zeronestakingkeeper.NewQualificationStakingKeeperAdapter(app.ZeroneStakingKeeper)
	app.QualificationKeeper = zeronequalificationkeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronequalificationtypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
		qualificationStakingAdapter,
	)
	// TODO: wire CaptureDefenseKeeper when x/capture_defense is available:
	// app.QualificationKeeper.SetCaptureDefenseKeeper(captureDefenseAdapter)
	app.QualificationKeeper.SetOntologyKeeper(&app.ZeroneOntologyKeeper)

	// Wire domain qualification into knowledge verification flow (R26-3).
	app.KnowledgeKeeper.SetDomainQualificationKeeper(
		zeronequalificationkeeper.NewKnowledgeDomainQualificationAdapter(app.QualificationKeeper),
	)

	emergencyStakingAdapter := zeronestakingkeeper.NewEmergencyStakingAdapter(app.ZeroneStakingKeeper)
	app.EmergencyKeeper = zeroneemergencykeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeroneemergencytypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		emergencyStakingAdapter,
	)
	app.ZeroneGovKeeper.SetEmergencyKeeper(zeroneemergencykeeper.NewGovEmergencyAdapter(app.EmergencyKeeper))

	// ---- Capture Defense + Capture Challenge keepers (R6-4) ----
	// capture_defense first (capture_challenge depends on it)
	app.CaptureDefenseKeeper = zeronecdkeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronecdtypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)
	app.CaptureDefenseKeeper.SetOntologyKeeper(&app.ZeroneOntologyKeeper)

	app.CaptureChallengeKeeper = zeronecckeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronecctypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
	)

	// R28-8: Wire capture defense immune system cross-module dependencies.

	// capture_defense → capture_challenge (auto-submit challenges when flagged)
	app.CaptureDefenseKeeper.SetChallengeKeeper(
		zeronecckeeper.NewCaptureDefenseAutoChallenger(app.CaptureChallengeKeeper),
	)

	// capture_challenge → capture_defense (read metrics, clear flags)
	app.CaptureChallengeKeeper.SetCaptureDefenseKeeper(
		zeronecdkeeper.NewChallengeCaptureDefenseAdapter(app.CaptureDefenseKeeper),
	)

	// capture_challenge → qualification (reduce weight on upheld challenge)
	app.CaptureChallengeKeeper.SetQualificationKeeper(app.QualificationKeeper)

	// capture_challenge → knowledge (increase threshold on upheld challenge)
	app.CaptureChallengeKeeper.SetKnowledgeKeeper(app.KnowledgeKeeper)

	// training_provenance reads from knowledge, qualification, and
	// capture_challenge to synthesize ProvenanceCertificates. Pure
	// consumer; no producers writing to it. The module is "a bundle
	// of edges, named" — it owns no state beyond the wiring.
	app.TrainingProvenanceKeeper = zeroneprovenancekeeper.NewKeeper(appCodec)
	app.TrainingProvenanceKeeper.SetKnowledgeKeeper(app.KnowledgeKeeper)
	app.TrainingProvenanceKeeper.SetQualificationKeeper(app.QualificationKeeper)
	app.TrainingProvenanceKeeper.SetCaptureChallengeKeeper(
		zeronecckeeper.NewTrainingProvenanceAdapter(app.CaptureChallengeKeeper),
	)

	// trust_score reads from knowledge (calibration), qualification
	// (metrics + penalties), and capture_challenge (strike count).
	// Same shape as training_provenance: pure consumer, no state.
	app.TrustScoreKeeper = zeronetrustscorekeeper.NewKeeper(appCodec)
	app.TrustScoreKeeper.SetKnowledgeKeeper(app.KnowledgeKeeper)
	app.TrustScoreKeeper.SetQualificationKeeper(app.QualificationKeeper)
	app.TrustScoreKeeper.SetCaptureChallengeKeeper(
		zeronecckeeper.NewTrustScoreAdapter(app.CaptureChallengeKeeper),
	)

	// governance_synthesis bundles system-stress signals from
	// knowledge (incidents/pauses/pending injections/privileged log),
	// capture_challenge (cartel posture), and alignment (autonomous-
	// throttle pacing). Per-system synthesizer; same shape.
	app.GovernanceSynthesisKeeper = zeronegovsynthkeeper.NewKeeper(appCodec)
	app.GovernanceSynthesisKeeper.SetKnowledgeKeeper(app.KnowledgeKeeper)
	app.GovernanceSynthesisKeeper.SetCaptureChallengeKeeper(
		zeronecckeeper.NewGovernanceSynthesisAdapter(app.CaptureChallengeKeeper),
	)
	app.GovernanceSynthesisKeeper.SetAlignmentKeeper(&app.AlignmentKeeper)
	// Frontier-query upstreams. Optional; absence degrades Frontier
	// to an empty list rather than failing. Wired AFTER
	// inquiry+counterexample keepers exist (their setters are below
	// the gov-synth construction; we re-apply the frontier setters
	// after those modules are constructed).

	// x/private_corpus: hash-anchor for off-chain vaults. The keeper is
	// stateful (vaults, manifests, access records) but does NOT read
	// from any other module — by design, see x/private_corpus/doc.go.
	app.PrivateCorpusKeeper = zeroneprivatecorpuskeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeroneprivatecorpustypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	// x/counterexamples: alignment-by-structure. Commitment 15 says
	// the corpus must include not just what is true but what is wrong
	// AND WHY. The keeper holds counterexample state and exposes
	// HasValidatedCounterexample to x/knowledge so TVW can apply the
	// alignment-by-structure multiplier.
	app.CounterexamplesKeeper = zeronecounterexkeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronecounterextypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)
	// Wire the existence-check from knowledge → counterexamples (so
	// counterexamples can refuse anchoring to non-existent facts) and
	// the validated-flag from counterexamples → knowledge (so TVW can
	// apply the multiplier).
	app.CounterexamplesKeeper.SetFactKeeper(
		zeroneknowledgekeeper.NewCounterexamplesFactAdapter(app.KnowledgeKeeper),
	)
	app.KnowledgeKeeper.SetCounterexampleKeeper(&app.CounterexamplesKeeper)

	// x/inquiry: open-question market for unmapped territory
	// (commitment 16). Asker escrows bounty into the inquiry-bounty-pool
	// module account; first answerer whose claim accepts wins. Bounty
	// returns on expiry or asker-cancel.
	app.InquiryKeeper = zeroneinquirykeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeroneinquirytypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
	)
	// Wire knowledge → inquiry so the auto-resolver can detect when
	// a linked claim has produced an accepted fact.
	app.InquiryKeeper.SetKnowledgeKeeper(
		zeroneknowledgekeeper.NewInquiryKnowledgeAdapter(app.KnowledgeKeeper),
	)

	// x/agent_understanding: pure read-only synthesizer composing
	// per-agent, per-domain understanding profiles. Wires adapters
	// from each upstream module that holds agent activity data.
	app.AgentUnderstandingKeeper = zeroneagentukeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeroneagentutypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)
	app.AgentUnderstandingKeeper.SetKnowledgeKeeper(
		zeroneknowledgekeeper.NewAgentUnderstandingKnowledgeAdapter(app.KnowledgeKeeper),
	)
	app.AgentUnderstandingKeeper.SetQualificationKeeper(
		zeronequalificationkeeper.NewAgentUnderstandingQualificationAdapter(app.QualificationKeeper),
	)
	app.AgentUnderstandingKeeper.SetCounterexamplesKeeper(
		zeronecounterexkeeper.NewAgentUnderstandingAdapter(app.CounterexamplesKeeper),
	)
	app.AgentUnderstandingKeeper.SetInquiryKeeper(
		zeroneinquirykeeper.NewAgentUnderstandingAdapter(app.InquiryKeeper),
	)

	// x/dialectic: pure read-only synthesizer composing per-fact,
	// per-domain, per-agent-pair disagreement signatures from
	// verification rounds.
	app.DialecticKeeper = zeronedialectickeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronedialectictypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)
	app.DialecticKeeper.SetKnowledgeKeeper(
		zeroneknowledgekeeper.NewDialecticAdapter(app.KnowledgeKeeper),
	)

	// Frontier-query wiring: now that inquiry, counterexamples, and
	// ontology keepers all exist, register the adapters with
	// governance_synthesis so its Frontier query can compose
	// per-domain sparsity from real chain state.
	app.GovernanceSynthesisKeeper.SetOntologyKeeper(
		zeroneontologykeeper.NewGovernanceSynthesisAdapter(app.ZeroneOntologyKeeper),
	)
	app.GovernanceSynthesisKeeper.SetFrontierKnowledgeKeeper(
		zeroneknowledgekeeper.NewGovernanceSynthesisFrontierAdapter(app.KnowledgeKeeper),
	)
	app.GovernanceSynthesisKeeper.SetFrontierInquiryKeeper(
		zeroneinquirykeeper.NewGovernanceSynthesisAdapter(app.InquiryKeeper),
	)
	app.GovernanceSynthesisKeeper.SetFrontierCounterexamplesKeeper(
		zeronecounterexkeeper.NewGovernanceSynthesisAdapter(app.CounterexamplesKeeper),
	)

	// knowledge → capture_defense (feed verification history + reputation)
	app.KnowledgeKeeper.SetCaptureDefenseKeeper(
		zeronecdkeeper.NewKnowledgeCaptureDefenseAdapter(app.CaptureDefenseKeeper),
	)

	// R31-4: capture_defense → knowledge (verification activity for HHI threshold relaxation)
	app.CaptureDefenseKeeper.SetKnowledgeKeeper(
		zeroneknowledgekeeper.NewCaptureDefenseKnowledgeAdapter(app.KnowledgeKeeper),
	)

	// alignment → capture_defense (read flagged domain count for security sensor)
	app.AlignmentKeeper.SetCaptureDefenseKeeper(
		zeronecdkeeper.NewAlignmentCaptureDefenseAdapter(app.CaptureDefenseKeeper),
	)

	// IBCRateLimitKeeper already created above (before TransferKeeper).

	app.ICAAuthKeeper = zeroneicaauthkeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[zeroneicaauthtypes.StoreKey]),
		app.ICAControllerKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	app.ResearchKeeper = zeroneresearchkeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeroneresearchtypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
	)

	// ---- Alignment keeper (R7-2) ----
	alignmentKnowledgeAdapter := zeroneknowledgekeeper.NewAlignmentKnowledgeAdapter(app.KnowledgeKeeper)
	alignmentStakingAdapter := zeronestakingkeeper.NewAlignmentStakingAdapter(app.ZeroneStakingKeeper)
	alignmentOntologyAdapter := zeroneontologykeeper.NewAlignmentOntologyAdapter(app.ZeroneOntologyKeeper)
	alignmentEmergencyAdapter := zeroneemergencykeeper.NewAlignmentEmergencyAdapter(app.EmergencyKeeper)
	alignmentVestingRewardsAdapter := vestingrewardskeeper.NewAlignmentVestingRewardsAdapter(app.VestingRewardsKeeper)
	app.AlignmentKeeper = zeronealignmentkeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronealignmenttypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		alignmentKnowledgeAdapter,
		alignmentStakingAdapter,
		alignmentOntologyAdapter,
		alignmentEmergencyAdapter,
		alignmentVestingRewardsAdapter,
	)
	// R29-6: Wire global pacing from alignment to consuming modules.
	alignmentPacingAdapter := zeronealignmentkeeper.NewAlignmentPacingAdapter(app.AlignmentKeeper)
	app.KnowledgeKeeper.SetPacingKeeper(alignmentPacingAdapter)
	app.CaptureDefenseKeeper.SetPacingKeeper(alignmentPacingAdapter)
	app.PartnershipsKeeper.SetPacingKeeper(alignmentPacingAdapter)
	app.DiscoveryKeeper.SetPacingKeeper(alignmentPacingAdapter)
	// ---- Autopoiesis keeper (R7-1) ----
	apStakingAdapter := zeronestakingkeeper.NewStakingForAutopoiesisAdapter(app.ZeroneStakingKeeper)
	app.AutopoiesisKeeper = zeroneapkeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeroneaptypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		apStakingAdapter,
	)
	// Post-init setters to break circular dependencies.
	apKnowledgeAdapter := zeroneknowledgekeeper.NewKnowledgeForAutopoiesisAdapter(app.KnowledgeKeeper)
	app.AutopoiesisKeeper.SetKnowledgeKeeper(apKnowledgeAdapter)
	app.AutopoiesisKeeper.SetEmergencyKeeper(&app.EmergencyKeeper)

	// Wire autopoiesis adapters into consuming modules.
	apForStaking := zeronestakingkeeper.NewAutopoiesisStakingAdapter(app.AutopoiesisKeeper)
	app.ZeroneStakingKeeper.SetAutopoiesisKeeper(apForStaking)
	apForKnowledge := zeroneknowledgekeeper.NewAutopoiesisKnowledgeAdapter(app.AutopoiesisKeeper)
	app.KnowledgeKeeper.SetAutopoiesisKeeper(apForKnowledge)
	apForVesting := vestingrewardskeeper.NewAutopoiesisVestingAdapter(app.AutopoiesisKeeper)
	app.VestingRewardsKeeper.SetAutopoiesisKeeper(apForVesting)
	// Knowledge-coupled block reward (T9 / thesis claim 1): block rewards scale
	// with verification throughput once this adapter is wired.
	app.VestingRewardsKeeper.SetKnowledgeKeeper(zeroneknowledgekeeper.NewVestingRewardsKnowledgeAdapter(app.KnowledgeKeeper))
	app.AlignmentKeeper.SetAutopoiesisKeeper(&app.AutopoiesisKeeper)

	// ---- Evidence Management keeper (R7-6) ----
	emStakingAdapter := zeronestakingkeeper.NewEvidenceMgmtStakingAdapter(app.ZeroneStakingKeeper)
	emDisputesAdapter := zeronedisputeskeeper.NewEvidenceMgmtDisputesAdapter(app.DisputesKeeper)
	app.EvidenceMgmtKeeper = zeroneemkeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeroneemtypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		emStakingAdapter,
		emDisputesAdapter,
	)

	// ---- Claiming Pot keeper (R7-6) ----
	cpotStakingAdapter := zeronestakingkeeper.NewClaimingPotStakingAdapter(app.ZeroneStakingKeeper)
	cpotAuthAdapter := zeroneauthkeeper.NewClaimingPotAuthAdapter(app.ZeroneAuthKeeper)
	app.ClaimingPotKeeper = zeronecpotkeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronecpottypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		cpotStakingAdapter,
		cpotAuthAdapter,
		app.BankKeeper,
	)

	// ---- Tree keeper (R7-5) ----
	treeRFDAdapter := vestingrewardskeeper.NewResearchFundDepositorAdapter(app.VestingRewardsKeeper)
	app.TreeKeeper = zeronetreekeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[zeronettreetypes.StoreKey]),
		app.BankKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		treeRFDAdapter,
	)
	// TODO: Wire channels keeper when x/channels implements GetChannelInfo/SpendFromChannel:
	// app.TreeKeeper.SetChannelsKeeper(channelsAdapter)

	// ---- Home keeper (R8-1) ----
	app.HomeKeeper = zeronehomekeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronehometypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
	)

	// ---- Partnerships keeper (R8-1) ----
	app.PartnershipsKeeper = zeronepartnershipskeeper.NewKeeper(
		appCodec,
		sdkruntime.NewKVStoreService(keys[zeronepartnershipstypes.StoreKey]),
		app.BankKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)
	// Break home↔partnerships circular dependency via setter.
	app.PartnershipsKeeper.SetHomeKeeper(app.HomeKeeper)

	// Wire partnership reward routing into knowledge verification flow (R26-4).
	app.KnowledgeKeeper.SetPartnershipKeeper(
		zeronepartnershipskeeper.NewKnowledgePartnershipAdapter(app.PartnershipsKeeper),
	)

	// Wire zerone auth into knowledge and partnerships for role bonuses (R28-5).
	knowledgeAuthAdapter := zeroneauthkeeper.NewKnowledgeAuthAdapter(app.ZeroneAuthKeeper)
	app.KnowledgeKeeper.SetZeroneAuthKeeper(knowledgeAuthAdapter)
	app.PartnershipsKeeper.SetZeroneAuthKeeper(knowledgeAuthAdapter)

	// R29-5: Wire structural immunity cross-module dependencies.
	// capture_defense → partnerships (read density, set formation bonus)
	app.CaptureDefenseKeeper.SetPartnershipsKeeper(
		zeronepartnershipskeeper.NewCaptureDefensePartnershipsAdapter(app.PartnershipsKeeper),
	)
	// partnerships → capture_defense (read flagged status)
	app.PartnershipsKeeper.SetCaptureDefenseKeeper(
		zeronecdkeeper.NewPartnershipsCaptureDefenseAdapter(app.CaptureDefenseKeeper),
	)

	// R31-4: Wire Metal→Water cross-module dependencies.
	// discovery → qualification (qualified domains for match scoring)
	app.DiscoveryKeeper.SetQualificationKeeper(app.QualificationKeeper)
	// partnerships → ontology (related strata for cross-stratum matching)
	app.PartnershipsKeeper.SetOntologyKeeper(&app.ZeroneOntologyKeeper)

	// R31-5: Wire Water → Wood — mentorship dividends flow to knowledge.
	app.PartnershipsKeeper.SetKnowledgeKeeper(
		zeronepartnershipskeeper.NewKnowledgeDividendAdapter(app.KnowledgeKeeper),
	)

	// R31-3: Wire alignment health signal into governance for expedited voting.
	app.ZeroneGovKeeper.SetAlignmentKeeper(&app.AlignmentKeeper)
	// R31-3: Wire partnerships keeper into governance for domain formation freezes.
	app.ZeroneGovKeeper.SetPartnershipsKeeper(
		zeronepartnershipskeeper.NewGovPartnershipsAdapter(app.PartnershipsKeeper),
	)

	// ---- Toolbox keeper (R8-1) ----
	toolboxRFDAdapter := vestingrewardskeeper.NewResearchFundDepositorAdapter(app.VestingRewardsKeeper)
	app.ToolboxKeeper = zeronetoolboxkeeper.NewKeeper(
		sdkruntime.NewKVStoreService(keys[zeronetoolboxtypes.StoreKey]),
		appCodec,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		app.BankKeeper,
		toolboxRFDAdapter,
	)
	// Wire optional cross-module keepers via setters.
	app.ToolboxKeeper.SetHomeKeeper(zeronehomekeeper.NewToolboxHomeAdapter(app.HomeKeeper))
	app.ToolboxKeeper.SetBillingKeeper(zeronebillingkeeper.NewToolboxBillingAdapter(app.BillingKeeper))
	// TODO: Wire remaining toolbox optional keepers when adapters are available:
	// app.ToolboxKeeper.SetDiscoveryKeeper(discoveryAdapter)
	// app.ToolboxKeeper.SetBvmKeeper(bvmAdapter)
	// app.ToolboxKeeper.SetKnowledgeKeeper(knowledgeAdapter)
	// app.ToolboxKeeper.SetStakingKeeper(stakingAdapter)

	// ---- IBC Router ----
	// SECURITY: Rate limit middleware wraps transfer module to prevent bridge drain attacks.
	transferIBCModule := ibctransfer.NewIBCModule(app.TransferKeeper)
	rateLimitMiddleware := zeroneibcratelimit.NewIBCMiddleware(
		transferIBCModule,
		app.IBCFeeKeeper, // ICS4Wrapper for SendPacket forwarding
		app.IBCRateLimitKeeper,
	)

	ibcRouter := ibcporttypes.NewRouter()
	ibcRouter.AddRoute(ibctransfertypes.ModuleName, rateLimitMiddleware)
	ibcRouter.AddRoute(
		icacontrollertypes.SubModuleName,
		icacontroller.NewIBCMiddleware(nil, app.ICAControllerKeeper),
	)
	ibcRouter.AddRoute(icahosttypes.SubModuleName, icahost.NewIBCModule(app.ICAHostKeeper))
	app.IBCKeeper.SetRouter(ibcRouter)

	// ---- Module Manager ----
	app.ModuleManager = module.NewManager(
		genutil.NewAppModule(app.AccountKeeper, app.StakingKeeper, app, txConfig),
		auth.NewAppModule(appCodec, app.AccountKeeper, nil, nil),
		vesting.NewAppModule(app.AccountKeeper, app.BankKeeper),
		bank.NewAppModule(appCodec, app.BankKeeper, app.AccountKeeper, nil),
		staking.NewAppModule(appCodec, app.StakingKeeper, app.AccountKeeper, app.BankKeeper, nil),
		distr.NewAppModule(appCodec, app.DistrKeeper, app.AccountKeeper, app.BankKeeper, app.StakingKeeper, nil),
		gov.NewAppModule(appCodec, app.GovKeeper, app.AccountKeeper, app.BankKeeper, nil),
		slashing.NewAppModule(appCodec, app.SlashingKeeper, app.AccountKeeper, app.BankKeeper, app.StakingKeeper, nil, appCodec.InterfaceRegistry()),
		feegrantmodule.NewAppModule(appCodec, app.AccountKeeper, app.BankKeeper, app.FeeGrantKeeper, appCodec.InterfaceRegistry()),
		upgrade.NewAppModule(app.UpgradeKeeper, addresscodec.NewBech32Codec(AccountAddressPrefix)),
		evidence.NewAppModule(app.EvidenceKeeper),
		consensus.NewAppModule(appCodec, app.ConsensusKeeper),
		capability.NewAppModule(appCodec, *app.CapabilityKeeper, false),
		ibc.NewAppModule(app.IBCKeeper),
		ibctransfer.NewAppModule(app.TransferKeeper),
		ibcfee.NewAppModule(app.IBCFeeKeeper),
		ica.NewAppModule(&app.ICAControllerKeeper, &app.ICAHostKeeper),
		// ===== Zerone custom modules — added by batch =====
		zeroneauth.NewAppModule(appCodec, app.ZeroneAuthKeeper),
		zeronestaking.NewAppModule(app.ZeroneStakingKeeper),
		vestingrewards.NewAppModule(appCodec, app.VestingRewardsKeeper),
		zeroneontology.NewAppModule(appCodec, app.ZeroneOntologyKeeper),
		zeroneknowledge.NewAppModule(appCodec, app.KnowledgeKeeper),
		zeronetokens.NewAppModule(appCodec, app.TokensKeeper),
		zeronebilling.NewAppModule(appCodec, app.BillingKeeper),
		zeroneliquiditypool.NewAppModule(appCodec, app.LiquidityPoolKeeper),
		zeronegov.NewAppModule(appCodec, app.ZeroneGovKeeper),
		zeronechannels.NewAppModule(appCodec, app.ChannelsKeeper),
		zeroneschedule.NewAppModule(appCodec, app.ScheduleKeeper),
		zeronecomputepool.NewAppModule(appCodec, app.ComputePoolKeeper),
		zeronediscovery.NewAppModule(appCodec, app.DiscoveryKeeper),
		zeronebvm.NewAppModule(appCodec, app.BVMKeeper),
		zeronedisputes.NewAppModule(appCodec, app.DisputesKeeper),
		zeronequalification.NewAppModule(appCodec, app.QualificationKeeper),
		zeroneemergency.NewAppModule(appCodec, app.EmergencyKeeper),
		zeronecapturedefense.NewAppModule(appCodec, app.CaptureDefenseKeeper),
		zeronecapturechallenge.NewAppModule(appCodec, app.CaptureChallengeKeeper),
		zeroneibcratelimit.NewAppModule(appCodec, app.IBCRateLimitKeeper),
		zeroneicaauth.NewAppModule(appCodec, app.ICAAuthKeeper),
		zeroneresearch.NewAppModule(appCodec, app.ResearchKeeper),
		zeroneprovenance.NewAppModule(appCodec, app.TrainingProvenanceKeeper),
		zeronetrustscore.NewAppModule(appCodec, app.TrustScoreKeeper),
		zeronegovsynth.NewAppModule(appCodec, app.GovernanceSynthesisKeeper),
		zeronealignment.NewAppModule(appCodec, app.AlignmentKeeper),
		zeroneautopoiesis.NewAppModule(appCodec, app.AutopoiesisKeeper),
		zeroneevidencemgmt.NewAppModule(appCodec, app.EvidenceMgmtKeeper),
		zeroneclaimingpot.NewAppModule(appCodec, app.ClaimingPotKeeper),
		zeronetree.NewAppModule(appCodec, app.TreeKeeper),               // R7-5: x/tree
		zeronehome.NewAppModule(appCodec, app.HomeKeeper),                // R8-1: x/home
		zeronepartnerships.NewAppModule(appCodec, app.PartnershipsKeeper), // R8-1: x/partnerships
		zeronetoolbox.NewAppModule(appCodec, app.ToolboxKeeper),          // R8-1: x/toolbox
		zeroneprivatecorpus.NewAppModule(appCodec, app.PrivateCorpusKeeper),
		zeronecounterex.NewAppModule(appCodec, app.CounterexamplesKeeper),
		zeroneinquiry.NewAppModule(appCodec, app.InquiryKeeper),
		zeroneagentu.NewAppModule(appCodec, app.AgentUnderstandingKeeper),
		zeronedialectic.NewAppModule(appCodec, app.DialecticKeeper),
	)

	app.ModuleManager.SetOrderBeginBlockers(
		upgradetypes.ModuleName,
		capabilitytypes.ModuleName,
		distrtypes.ModuleName,
		slashingtypes.ModuleName,
		evidencetypes.ModuleName,
		stakingtypes.ModuleName,
		authtypes.ModuleName,
		banktypes.ModuleName,
		govtypes.ModuleName,
		genutiltypes.ModuleName,
		vestingtypes.ModuleName,
		feegrant.ModuleName,
		ibcexported.ModuleName,
		ibctransfertypes.ModuleName,
		ibcfeetypes.ModuleName,
		icatypes.ModuleName,
		// ===== Zerone custom module BeginBlocker order — added by batch =====
		zeroneemergencytypes.ModuleName,        // emergency: EARLY — ceremony progress, auto-resume, revert monitoring
		vestingrewardstypes.ModuleName, // MUST run before x/distribution to intercept fees
		zeroneauthtypes.ModuleName,
		zeronestakingtypes.ModuleName,
		zeronegovtypes.ModuleName,       // gov: after staking (needs bonded stake)
		zeroneontologytypes.ModuleName,
		zeroneknowledgetypes.ModuleName, // LAST: depends on staking + ontology state
		zeronetokenstypes.ModuleName,    // tokens: emission period processing
		zeronebillingtypes.ModuleName,   // billing: no-op
		zeronelptypes.ModuleName,        // liquiditypool: TWAP accumulator updates
		zeronechannelstypes.ModuleName,      // channels: auto-settle expired + periodic settlements
		zeronescheduletypes.ModuleName,      // schedule: no-op in BeginBlock
		zeronecptypes.ModuleName,            // compute_pool: jail inactive, price changes, unbonding
		zeronediscoverytypes.ModuleName,     // discovery: expire stale profiles
		zeronebvmtypes.ModuleName,           // bvm: execute pending scheduled contracts
		zeronehometypes.ModuleName,          // home: deadman switches, session cleanup
		zeronedisputestypes.ModuleName,              // disputes: phase transitions based on deadlines
		zeronequalificationtypes.ModuleName,         // qualification: expiry, promotion, stake unlock
		zeronecdtypes.ModuleName,                    // capture_defense: decay + auto-analysis (before challenge)
		zeronecctypes.ModuleName,                    // capture_challenge: phase advancement, risk analysis
		zeroneresearchtypes.ModuleName,              // research: bounty expiry
		zeroneprovenancetypes.ModuleName,            // training_provenance: no-op in BeginBlock (pure synthesizer)
		zeronetrustscoretypes.ModuleName,            // trust_score: no-op in BeginBlock (pure synthesizer)
		zeronegovsynthtypes.ModuleName,              // governance_synthesis: no-op in BeginBlock (pure synthesizer)
		zeronealignmenttypes.ModuleName,             // alignment: no-op in BeginBlock
		zeroneaptypes.ModuleName,                    // autopoiesis: no-op in BeginBlock
		zeroneibcrltypes.ModuleName,                 // ibcratelimit: reset expired windows
		zeroneicaauthtypes.ModuleName,               // icaauth: no-op
		zeronepartnershipstypes.ModuleName,          // partnerships: expire formations, lift freezes
		zeroneemtypes.ModuleName,                    // evidence_mgmt: no-op (event-driven)
		zeronecpottypes.ModuleName,                  // claiming_pot: pot expiry
		zeronettreetypes.ModuleName,                 // tree: expire seeds past expiry block
		zeronetoolboxtypes.ModuleName,               // toolbox: no-op BeginBlock
		zeroneprivatecorpustypes.ModuleName,         // private_corpus: no-op BeginBlock (operator-driven)
		zeronecounterextypes.ModuleName,             // counterexamples: no-op BeginBlock (proposal-driven)
		zeroneinquirytypes.ModuleName,               // inquiry: scan OPEN/ANSWERED inquiries for resolution + expiry
		zeroneagentutypes.ModuleName,                // agent_understanding: no-op BeginBlock (pure synthesizer)
		zeronedialectictypes.ModuleName,             // dialectic: no-op BeginBlock (pure synthesizer)
	)

	app.ModuleManager.SetOrderEndBlockers(
		govtypes.ModuleName,
		stakingtypes.ModuleName,
		ibcexported.ModuleName,
		ibctransfertypes.ModuleName,
		ibcfeetypes.ModuleName,
		icatypes.ModuleName,
		capabilitytypes.ModuleName,
		authtypes.ModuleName,
		banktypes.ModuleName,
		distrtypes.ModuleName,
		slashingtypes.ModuleName,
		evidencetypes.ModuleName,
		feegrant.ModuleName,
		genutiltypes.ModuleName,
		vestingtypes.ModuleName,
		// ===== Zerone custom module EndBlocker order — added by batch =====
		zeroneauthtypes.ModuleName,
		zeronestakingtypes.ModuleName,
		zeronegovtypes.ModuleName,       // EndBlocker: no-op
		vestingrewardstypes.ModuleName,
		zeroneontologytypes.ModuleName,  // EndBlocker: expire proposals
		zeroneknowledgetypes.ModuleName, // EndBlocker: no-op for now
		zeronetokenstypes.ModuleName,    // EndBlocker: no-op
		zeronebillingtypes.ModuleName,   // EndBlocker: no-op
		zeronelptypes.ModuleName,        // EndBlocker: no-op
		zeronechannelstypes.ModuleName,      // EndBlocker: no-op
		zeronescheduletypes.ModuleName,      // EndBlocker: process due schedules
		zeronecptypes.ModuleName,            // EndBlocker: no-op
		zeronediscoverytypes.ModuleName,     // EndBlocker: no-op
		zeronebvmtypes.ModuleName,           // EndBlocker: no-op
		zeronehometypes.ModuleName,          // EndBlocker: cleanup old acknowledged alerts
		zeronedisputestypes.ModuleName,              // EndBlocker: no-op
		zeronequalificationtypes.ModuleName,         // EndBlocker: no-op
		zeroneemergencytypes.ModuleName,             // EndBlocker: epoch counter reset
		zeroneaptypes.ModuleName,                    // EndBlocker: epoch SSI processing + multiplier adjustment
		zeronecdtypes.ModuleName,                    // EndBlocker: no-op
		zeronecctypes.ModuleName,                    // EndBlocker: no-op
		zeroneresearchtypes.ModuleName,              // EndBlocker: no-op
		zeroneprovenancetypes.ModuleName,            // EndBlocker: no-op
		zeronetrustscoretypes.ModuleName,            // EndBlocker: no-op
		zeronegovsynthtypes.ModuleName,              // EndBlocker: no-op
		zeronealignmenttypes.ModuleName,             // EndBlocker: observation→scoring→corrections at interval
		zeroneibcrltypes.ModuleName,                 // EndBlocker: no-op
		zeroneicaauthtypes.ModuleName,               // EndBlocker: no-op
		zeronepartnershipstypes.ModuleName,          // EndBlocker: settle cooling partnerships
		zeroneemtypes.ModuleName,                    // EndBlocker: no-op
		zeronecpottypes.ModuleName,                  // EndBlocker: no-op
		zeronettreetypes.ModuleName,                 // EndBlocker: no-op
		zeronetoolboxtypes.ModuleName,               // EndBlocker: no-op
		zeroneprivatecorpustypes.ModuleName,         // EndBlocker: no-op
		zeronecounterextypes.ModuleName,             // EndBlocker: no-op
		zeroneinquirytypes.ModuleName,               // EndBlocker: no-op
		zeroneagentutypes.ModuleName,                // EndBlocker: no-op
		zeronedialectictypes.ModuleName,             // EndBlocker: no-op
	)

	genesisOrder := []string{
		capabilitytypes.ModuleName,
		authtypes.ModuleName,
		banktypes.ModuleName,
		distrtypes.ModuleName,
		stakingtypes.ModuleName,
		slashingtypes.ModuleName,
		govtypes.ModuleName,
		feegrant.ModuleName,
		evidencetypes.ModuleName,
		ibcexported.ModuleName,
		genutiltypes.ModuleName,
		ibctransfertypes.ModuleName,
		ibcfeetypes.ModuleName,
		icatypes.ModuleName,
		vestingtypes.ModuleName,
		upgradetypes.ModuleName,
		// ===== Zerone custom module genesis order — added by batch =====
		zeroneauthtypes.ModuleName,
		zeronestakingtypes.ModuleName,
		zeronegovtypes.ModuleName,       // Genesis: after staking (needs staking data for quorum)
		vestingrewardstypes.ModuleName,
		zeroneontologytypes.ModuleName,  // Genesis: after bank (needs bank for stake escrow)
		zeroneknowledgetypes.ModuleName, // Genesis: after ontology + staking (needs both)
		zeronetokenstypes.ModuleName,    // Genesis: after bank (needs bank for wrap)
		zeronebillingtypes.ModuleName,   // Genesis: after knowledge (depends on knowledge for fact queries)
		zeronelptypes.ModuleName,        // Genesis: after bank (needs bank for LP minting)
		zeronechannelstypes.ModuleName,      // Genesis: after bank (needs bank for escrow)
		zeronescheduletypes.ModuleName,      // Genesis: after bank (needs bank for fee escrow)
		zeronecptypes.ModuleName,            // Genesis: after bank (needs bank for stake escrow)
		zeronediscoverytypes.ModuleName,     // Genesis: after bank (needs bank for stake escrow)
		zeronebvmtypes.ModuleName,           // Genesis: after knowledge (needs knowledge for bridge)
		zeronehometypes.ModuleName,          // Genesis: after bank (needs bank for sends)
		zeronedisputestypes.ModuleName,              // Genesis: after knowledge + staking (needs both)
		zeronequalificationtypes.ModuleName,         // Genesis: after disputes + staking
		zeroneemergencytypes.ModuleName,             // Genesis: after staking (needs guardian tier info)
		zeroneaptypes.ModuleName,                    // Genesis: after emergency + knowledge + staking
		zeronecdtypes.ModuleName,                    // Genesis: after knowledge + staking
		zeronecctypes.ModuleName,                    // Genesis: after capture_defense
		zeroneresearchtypes.ModuleName,              // Genesis: after knowledge + bank (needs both)
		zeroneprovenancetypes.ModuleName,            // Genesis: after knowledge + qualification + capture_challenge (pure read consumer)
		zeronetrustscoretypes.ModuleName,            // Genesis: after knowledge + qualification + capture_challenge (pure read consumer)
		zeronegovsynthtypes.ModuleName,              // Genesis: after knowledge + capture_challenge + alignment (pure read consumer)
		zeronealignmenttypes.ModuleName,             // Genesis: after emergency + staking + knowledge (needs all)
		zeronepartnershipstypes.ModuleName,          // Genesis: after home (needs home for partnership links)
		zeroneibcrltypes.ModuleName,                 // Genesis: after IBC
		zeroneicaauthtypes.ModuleName,               // Genesis: after ICA
		zeroneemtypes.ModuleName,                    // Genesis: after disputes + staking
		zeronecpottypes.ModuleName,                  // Genesis: after staking + auth + bank
		zeronettreetypes.ModuleName,                 // Genesis: after bank + channels + vesting_rewards
		zeronetoolboxtypes.ModuleName,               // Genesis: after discovery + billing + home + tree (needs all)
		zeroneprivatecorpustypes.ModuleName,         // Genesis: standalone, no cross-module deps
		zeronecounterextypes.ModuleName,             // Genesis: after knowledge (uses fact-existence adapter)
		zeroneinquirytypes.ModuleName,               // Genesis: after knowledge (auto-resolver reads facts)
		zeroneagentutypes.ModuleName,                // Genesis: after all upstream modules (pure synthesizer)
		zeronedialectictypes.ModuleName,             // Genesis: after knowledge (pure synthesizer)
	}
	app.ModuleManager.SetOrderInitGenesis(genesisOrder...)
	app.ModuleManager.SetOrderExportGenesis(genesisOrder...)

	app.configurator = module.NewConfigurator(app.appCodec, app.MsgServiceRouter(), app.GRPCQueryRouter())
	if err := app.ModuleManager.RegisterServices(app.configurator); err != nil {
		panic(fmt.Sprintf("failed to register module services: %s", err))
	}

	// Register upgrade handlers (must be after RegisterServices, before LoadLatestVersion).
	app.RegisterUpgradeHandlers()

	// Configure store loaders for upgrades that add/remove store keys (must be before LoadLatestVersion).
	app.RegisterStoreUpgrades()

	// Mount stores
	app.MountKVStores(keys)
	app.MountTransientStores(tkeys)
	app.MountMemoryStores(memKeys)

	// Set ante handler
	app.SetAnteHandler(NewAnteHandler(app))

	// Set block handlers
	app.SetInitChainer(app.InitChainer)
	app.SetPreBlocker(app.PotPreBlocker)
	app.SetBeginBlocker(app.BeginBlocker)
	app.SetEndBlocker(app.EndBlocker)

	// ABCI++ handlers for Proof of Truth vote extensions
	app.SetPrepareProposal(app.PrepareProposalHandler())
	app.SetProcessProposal(app.ProcessProposalHandler())
	app.SetExtendVoteHandler(app.ExtendVoteHandler())
	app.SetVerifyVoteExtensionHandler(app.VerifyVoteExtensionHandler())

	// Wire oracle client if configured via app.toml [oracle] section.
	oracleEnabled := cast.ToBool(appOpts.Get("oracle.enabled"))
	if oracleEnabled {
		oracleEndpoint := cast.ToString(appOpts.Get("oracle.endpoint"))
		oracleTimeout := cast.ToDuration(appOpts.Get("oracle.timeout"))
		oracleMinConf := cast.ToFloat64(appOpts.Get("oracle.min-confidence"))
		if oracleEndpoint != "" {
			logger.Info("oracle sidecar enabled",
				"endpoint", oracleEndpoint,
				"timeout", oracleTimeout,
				"min_confidence", oracleMinConf,
			)
			app.oracleClient = NewHTTPOracleClient(oracleEndpoint, oracleTimeout, oracleMinConf)
		}
	}

	if loadLatest {
		if err := app.LoadLatestVersion(); err != nil {
			logger.Error("error loading latest version", "err", err)
			os.Exit(1)
		}

		}

	return app
}

// InitChainer initializes the chain from genesis.
func (app *ZeroneApp) InitChainer(ctx sdk.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
	var genesisState GenesisState
	if err := json.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
		panic(err)
	}

	// Ensure ZRN denomination metadata is present in the bank genesis state.
	// This registers the denom units (uzrn / mzrn / zrn) with the bank module
	// so queries like /cosmos/bank/v1beta1/denoms_metadata return useful data.
	if raw, ok := genesisState[banktypes.ModuleName]; ok {
		var bankGenState banktypes.GenesisState
		app.appCodec.MustUnmarshalJSON(raw, &bankGenState)
		if !hasZRNMetadata(bankGenState.DenomMetadata) {
			bankGenState.DenomMetadata = append(bankGenState.DenomMetadata, zrnDenomMetadata())
			genesisState[banktypes.ModuleName] = app.appCodec.MustMarshalJSON(&bankGenState)
		}
	}

	app.UpgradeKeeper.SetModuleVersionMap(ctx, app.ModuleManager.GetVersionMap())
	resp, err := app.ModuleManager.InitGenesis(ctx, app.appCodec, genesisState)
	if err != nil {
		return nil, err
	}

	// Write a sentinel key to every IAVL store so that none remain empty.
	// Empty IAVL stores cause CacheMultiStoreWithVersion to fail because
	// GetImmutable returns ErrVersionDoesNotExist for trees with a nil root.
	for _, key := range app.keys {
		store := ctx.KVStore(key)
		if !store.Has([]byte("_iavl_init")) {
			store.Set([]byte("_iavl_init"), []byte{0x01})
		}
	}

	return resp, nil
}

// zrnDenomMetadata returns the canonical ZRN token denomination metadata.
func zrnDenomMetadata() banktypes.Metadata {
	return banktypes.Metadata{
		Description: "The native staking and governance token of Zerone",
		DenomUnits: []*banktypes.DenomUnit{
			{Denom: "uzrn", Exponent: 0, Aliases: []string{"microzrn"}},
			{Denom: "mzrn", Exponent: 3, Aliases: []string{"millizrn"}},
			{Denom: "zrn", Exponent: 6, Aliases: nil},
		},
		Base:    "uzrn",
		Display: "zrn",
		Name:    "Zerone",
		Symbol:  "ZRN",
	}
}

// hasZRNMetadata checks if ZRN denom metadata is already present.
func hasZRNMetadata(metadata []banktypes.Metadata) bool {
	for _, m := range metadata {
		if m.Base == "uzrn" {
			return true
		}
	}
	return false
}

// BeginBlocker runs begin-block logic for all modules.
func (app *ZeroneApp) BeginBlocker(ctx sdk.Context) (sdk.BeginBlock, error) {
	return app.ModuleManager.BeginBlock(ctx)
}

// EndBlocker runs end-block logic for all modules.
func (app *ZeroneApp) EndBlocker(ctx sdk.Context) (sdk.EndBlock, error) {
	return app.ModuleManager.EndBlock(ctx)
}

// LoadHeight loads a specific application state height.
func (app *ZeroneApp) LoadHeight(height int64) error {
	return app.LoadVersion(height)
}

// AppCodec returns the protobuf codec.
func (app *ZeroneApp) AppCodec() codec.Codec {
	return app.appCodec
}

// InterfaceRegistry returns the interface registry.
func (app *ZeroneApp) InterfaceRegistry() codectypes.InterfaceRegistry {
	return app.interfaceRegistry
}

// TxConfig returns the transaction config.
func (app *ZeroneApp) TxConfig() client.TxConfig {
	return app.txConfig
}

// LegacyAmino returns the legacy amino codec.
func (app *ZeroneApp) LegacyAmino() *codec.LegacyAmino {
	return app.legacyAmino
}

// DefaultGenesis returns the default genesis state for all modules.
func (app *ZeroneApp) DefaultGenesis() GenesisState {
	return ModuleBasics.DefaultGenesis(app.appCodec)
}

// SimulationManager returns the simulation manager.
func (app *ZeroneApp) SimulationManager() *module.SimulationManager {
	return app.sm
}

// RegisterAPIRoutes registers REST API routes.
func (app *ZeroneApp) RegisterAPIRoutes(apiSvr *api.Server, apiConfig config.APIConfig) {
	clientCtx := apiSvr.ClientCtx
	authtypes.RegisterInterfaces(clientCtx.InterfaceRegistry)
	ModuleBasics.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// Register gRPC-gateway v2 routes for all Zerone custom modules.
	// The generated query.pb.gw.go files use grpc-gateway/v2, while the SDK's
	// GRPCGatewayRouter uses v1 — incompatible types. We create a separate v2
	// mux and mount it for /zerone/ paths.
	gwmux := gwv2runtime.NewServeMux()
	ctx := context.Background()
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	must(zeroneauthtypes.RegisterQueryHandlerClient(ctx, gwmux, zeroneauthtypes.NewQueryClient(clientCtx)))
	must(zeroneknowledgetypes.RegisterQueryHandlerClient(ctx, gwmux, zeroneknowledgetypes.NewQueryClient(clientCtx)))
	must(zeroneontologytypes.RegisterQueryHandlerClient(ctx, gwmux, zeroneontologytypes.NewQueryClient(clientCtx)))
	must(zeronestakingtypes.RegisterQueryHandlerClient(ctx, gwmux, zeronestakingtypes.NewQueryClient(clientCtx)))
	must(zeronebillingtypes.RegisterQueryHandlerClient(ctx, gwmux, zeronebillingtypes.NewQueryClient(clientCtx)))
	must(zeronelptypes.RegisterQueryHandlerClient(ctx, gwmux, zeronelptypes.NewQueryClient(clientCtx)))
	must(zeronetokenstypes.RegisterQueryHandlerClient(ctx, gwmux, zeronetokenstypes.NewQueryClient(clientCtx)))
	must(zeronechannelstypes.RegisterQueryHandlerClient(ctx, gwmux, zeronechannelstypes.NewQueryClient(clientCtx)))
	must(zeronegovtypes.RegisterQueryHandlerClient(ctx, gwmux, zeronegovtypes.NewQueryClient(clientCtx)))
	must(zeronehometypes.RegisterQueryHandlerClient(ctx, gwmux, zeronehometypes.NewQueryClient(clientCtx)))
	must(zeronepartnershipstypes.RegisterQueryHandlerClient(ctx, gwmux, zeronepartnershipstypes.NewQueryClient(clientCtx)))
	must(zeronescheduletypes.RegisterQueryHandlerClient(ctx, gwmux, zeronescheduletypes.NewQueryClient(clientCtx)))
	must(zeronecptypes.RegisterQueryHandlerClient(ctx, gwmux, zeronecptypes.NewQueryClient(clientCtx)))
	must(zeronediscoverytypes.RegisterQueryHandlerClient(ctx, gwmux, zeronediscoverytypes.NewQueryClient(clientCtx)))
	must(zeronebvmtypes.RegisterQueryHandlerClient(ctx, gwmux, zeronebvmtypes.NewQueryClient(clientCtx)))
	must(vestingrewardstypes.RegisterQueryHandlerClient(ctx, gwmux, vestingrewardstypes.NewQueryClient(clientCtx)))
	must(zeronedisputestypes.RegisterQueryHandlerClient(ctx, gwmux, zeronedisputestypes.NewQueryClient(clientCtx)))
	must(zeronequalificationtypes.RegisterQueryHandlerClient(ctx, gwmux, zeronequalificationtypes.NewQueryClient(clientCtx)))
	must(zeroneemergencytypes.RegisterQueryHandlerClient(ctx, gwmux, zeroneemergencytypes.NewQueryClient(clientCtx)))
	must(zeroneibcrltypes.RegisterQueryHandlerClient(ctx, gwmux, zeroneibcrltypes.NewQueryClient(clientCtx)))
	must(zeroneicaauthtypes.RegisterQueryHandlerClient(ctx, gwmux, zeroneicaauthtypes.NewQueryClient(clientCtx)))
	must(zeronecdtypes.RegisterQueryHandlerClient(ctx, gwmux, zeronecdtypes.NewQueryClient(clientCtx)))
	must(zeronecctypes.RegisterQueryHandlerClient(ctx, gwmux, zeronecctypes.NewQueryClient(clientCtx)))
	must(zeronealignmenttypes.RegisterQueryHandlerClient(ctx, gwmux, zeronealignmenttypes.NewQueryClient(clientCtx)))
	must(zeroneresearchtypes.RegisterQueryHandlerClient(ctx, gwmux, zeroneresearchtypes.NewQueryClient(clientCtx)))
	must(zeroneaptypes.RegisterQueryHandlerClient(ctx, gwmux, zeroneaptypes.NewQueryClient(clientCtx)))
	must(zeroneemtypes.RegisterQueryHandlerClient(ctx, gwmux, zeroneemtypes.NewQueryClient(clientCtx)))
	must(zeronecpottypes.RegisterQueryHandlerClient(ctx, gwmux, zeronecpottypes.NewQueryClient(clientCtx)))
	must(zeronetoolboxtypes.RegisterQueryHandlerClient(ctx, gwmux, zeronetoolboxtypes.NewQueryClient(clientCtx)))
	must(zeronettreetypes.RegisterQueryHandlerClient(ctx, gwmux, zeronettreetypes.NewQueryClient(clientCtx)))
	apiSvr.Router.PathPrefix("/zerone/").Handler(gwmux)

	// Swagger UI placeholder — full OpenAPI served from proto-generated spec (R10-2)
	if apiConfig.Swagger {
		RegisterSwaggerAPI(apiSvr.Router)
	}
}

// RegisterSwaggerAPI registers a Swagger UI route with the API router.
// Visit http://localhost:1317/swagger/ to view the interactive API docs.
func RegisterSwaggerAPI(rtr *mux.Router) {
	rtr.PathPrefix("/swagger/").Handler(
		http.StripPrefix("/swagger/", http.FileServer(http.FS(swagger.SwaggerUI))),
	)
}

// RegisterTxService implements the Application.RegisterTxService method.
func (app *ZeroneApp) RegisterTxService(clientCtx client.Context) {
	authtx.RegisterTxService(app.BaseApp.GRPCQueryRouter(), clientCtx, app.BaseApp.Simulate, app.interfaceRegistry)
}

// RegisterTendermintService implements the Application.RegisterTendermintService method.
func (app *ZeroneApp) RegisterTendermintService(clientCtx client.Context) {
	cmtservice.RegisterTendermintService(
		clientCtx,
		app.BaseApp.GRPCQueryRouter(),
		app.interfaceRegistry,
		app.Query,
	)
}

// RegisterNodeService implements the Application.RegisterNodeService method.
func (app *ZeroneApp) RegisterNodeService(clientCtx client.Context, cfg config.Config) {
	nodeservice.RegisterNodeService(clientCtx, app.GRPCQueryRouter(), cfg)
}

// blockedModuleAccountAddrs returns the set of module account addresses that
// are blocked from receiving funds (all module accounts except governance).
func blockedModuleAccountAddrs() map[string]bool {
	modAccAddrs := make(map[string]bool)
	for acc := range maccPerms {
		modAccAddrs[authtypes.NewModuleAddress(acc).String()] = true
	}
	// Allow governance module to receive funds (for proposal deposits).
	delete(modAccAddrs, authtypes.NewModuleAddress(govtypes.ModuleName).String())
	return modAccAddrs
}

// skipUpgradeHeights reads skip-upgrade-heights from app options.
func skipUpgradeHeights(appOpts servertypes.AppOptions) map[int64]bool {
	skipHeights := map[int64]bool{}
	for _, h := range cast.ToIntSlice(appOpts.Get(server.FlagUnsafeSkipUpgrades)) {
		skipHeights[int64(h)] = true
	}
	return skipHeights
}

// Ensure ZeroneApp implements the servertypes.Application interface at compile time.
var _ servertypes.Application = (*ZeroneApp)(nil)

// Suppress unused-import warnings for types that will be used by custom modules.
var (
	_ = govv1beta1.RegisterInterfaces
)
