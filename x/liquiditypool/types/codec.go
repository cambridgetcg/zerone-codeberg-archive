package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreatePool{}, "liquiditypool/CreatePool", nil)
	cdc.RegisterConcrete(&MsgSwap{}, "liquiditypool/Swap", nil)
	cdc.RegisterConcrete(&MsgAddLiquidity{}, "liquiditypool/AddLiquidity", nil)
	cdc.RegisterConcrete(&MsgRemoveLiquidity{}, "liquiditypool/RemoveLiquidity", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "liquiditypool/UpdateParams", nil)
	cdc.RegisterConcrete(&MsgCreateLiquidityMiningPeriod{}, "liquiditypool/CreateLiquidityMiningPeriod", nil)
	cdc.RegisterConcrete(&MsgClaimLiquidityRewards{}, "liquiditypool/ClaimLiquidityRewards", nil)
	cdc.RegisterConcrete(&MsgCommitSwap{}, "liquiditypool/CommitSwap", nil)
	cdc.RegisterConcrete(&MsgRevealSwap{}, "liquiditypool/RevealSwap", nil)
	cdc.RegisterConcrete(&MsgFundILInsurance{}, "liquiditypool/FundILInsurance", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreatePool{},
		&MsgSwap{},
		&MsgAddLiquidity{},
		&MsgRemoveLiquidity{},
		&MsgUpdateParams{},
		&MsgCreateLiquidityMiningPeriod{},
		&MsgClaimLiquidityRewards{},
		&MsgCommitSwap{},
		&MsgRevealSwap{},
		&MsgFundILInsurance{},
	)
}

var (
	Amino     = codec.NewLegacyAmino()
	ModuleCdc = codec.NewProtoCodec(cdctypes.NewInterfaceRegistry())
)
