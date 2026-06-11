package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// RegisterCodec registers module types with the legacy amino codec.
func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterValidator{}, "zerone/staking/MsgRegisterValidator", nil)
	cdc.RegisterConcrete(&MsgDelegate{}, "zerone/staking/MsgDelegate", nil)
	cdc.RegisterConcrete(&MsgUndelegate{}, "zerone/staking/MsgUndelegate", nil)
	cdc.RegisterConcrete(&MsgRedelegate{}, "zerone/staking/MsgRedelegate", nil)
	cdc.RegisterConcrete(&MsgUpdateValidatorStake{}, "zerone/staking/MsgUpdateValidatorStake", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "zerone/staking/MsgUpdateParams", nil)
	cdc.RegisterConcrete(&MsgSubmitSlashEvidence{}, "zerone/staking/MsgSubmitSlashEvidence", nil)
	cdc.RegisterConcrete(&MsgContributeToInsuranceFund{}, "zerone/staking/MsgContributeToInsuranceFund", nil)
	// Slashing Gap 1: Tombstoning
	cdc.RegisterConcrete(&MsgUntombstoneValidator{}, "zerone/staking/MsgUntombstoneValidator", nil)
}

// RegisterInterfaces registers module types with the interface registry.
func RegisterInterfaces(registry codectypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterValidator{},
		&MsgDelegate{},
		&MsgUndelegate{},
		&MsgRedelegate{},
		&MsgUpdateValidatorStake{},
		&MsgUpdateParams{},
		&MsgSubmitSlashEvidence{},
		&MsgContributeToInsuranceFund{},
		// Slashing Gap 1: Tombstoning
		&MsgUntombstoneValidator{},
	)
}
