package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVersionedResponsesMatchSchemas(t *testing.T) {
	handler := Handler()
	v1Schema, err := loadSchema("order.v1.json")
	require.NoError(t, err)
	v2Schema, err := loadSchema("order.v2.json")
	require.NoError(t, err)

	require.NoError(t, validate(v1Schema, get(handler, "/v1/orders/o-1")))
	require.NoError(t, validate(v2Schema, get(handler, "/v2/orders/o-1")))
}

func TestAdditiveFieldBreaksStrictV1Contract(t *testing.T) {
	handler := Handler()
	v1Schema, err := loadSchema("order.v1.json")
	require.NoError(t, err)
	require.Error(t, validate(v1Schema, get(handler, "/v2/orders/o-1")))
}

func TestNewContractRejectsOldResponse(t *testing.T) {
	handler := Handler()
	v2Schema, err := loadSchema("order.v2.json")
	require.NoError(t, err)
	require.Error(t, validate(v2Schema, get(handler, "/v1/orders/o-1")))
}
