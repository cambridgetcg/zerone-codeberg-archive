package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// RegisterCodec registers the oracle_committee module types with the legacy amino codec.
func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterOracleOperator{}, "zerone_oracle_committee/RegisterOracleOperator", nil)
	cdc.RegisterConcrete(&MsgSubmitOracleEvaluation{}, "zerone_oracle_committee/SubmitOracleEvaluation", nil)
	cdc.RegisterConcrete(&MsgDeregisterOracleOperator{}, "zerone_oracle_committee/DeregisterOracleOperator", nil)
	cdc.RegisterConcrete(&MsgUpdateOracleCommitteeParams{}, "zerone_oracle_committee/UpdateOracleCommitteeParams", nil)
}

// RegisterInterfaces registers the oracle_committee module interface implementations.
func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterOracleOperator{},
		&MsgSubmitOracleEvaluation{},
		&MsgDeregisterOracleOperator{},
		&MsgUpdateOracleCommitteeParams{},
	)
}
