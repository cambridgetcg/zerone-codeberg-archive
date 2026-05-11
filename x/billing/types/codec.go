package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// RegisterCodec registers the billing module types with the legacy amino codec.
func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterProvider{}, "zerone_billing/RegisterProvider", nil)
	cdc.RegisterConcrete(&MsgDeregisterProvider{}, "zerone_billing/DeregisterProvider", nil)
	cdc.RegisterConcrete(&MsgQueryFact{}, "zerone_billing/QueryFact", nil)
	cdc.RegisterConcrete(&MsgBatchQueryFacts{}, "zerone_billing/BatchQueryFacts", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "zerone_billing/UpdateParams", nil)
	cdc.RegisterConcrete(&MsgDevelopmentFundSpend{}, "zerone_billing/DevelopmentFundSpend", nil)
}

// RegisterInterfaces registers the billing module interface implementations.
func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterProvider{},
		&MsgDeregisterProvider{},
		&MsgQueryFact{},
		&MsgBatchQueryFacts{},
		&MsgUpdateParams{},
		&MsgDevelopmentFundSpend{},
	)
}
