package dto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCustomMenuItemsPreservesUserContextCompatibility(t *testing.T) {
	items := ParseCustomMenuItems(`[
		{"id":"legacy","label":"Legacy","url":"https://legacy.example.com","visibility":"user","sort_order":0},
		{"id":"isolated","label":"Canvas","url":"https://canvas.vote520.com","visibility":"admin","sort_order":1,"pass_user_context":false}
	]`)

	require.Len(t, items, 2)
	require.Nil(t, items[0].PassUserContext)
	require.NotNil(t, items[1].PassUserContext)
	require.False(t, *items[1].PassUserContext)
}
