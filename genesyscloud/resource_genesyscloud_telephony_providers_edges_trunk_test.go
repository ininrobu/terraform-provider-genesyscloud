package genesyscloud

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"testing"
)

func TestAccResourceTrunk(t *testing.T) {
	var (
		// trunk base settings used to create the trunk
		trunkBaseSettingsRes = "trunkBaseSettingsRes"

		//edge groups
		edgeGroupRes1 = "edgeGroupRes1"
		edgeGroupRes2 = "edgeGroupRes2"

		// trunk base settings of type PHONE needed for creating edge groups
		phoneTrunkBaseSettingsRes = "phoneTrunkBaseSettingsRes"

		// trunk
		trunkRes = "trunkRes"
	)

	phoneTrunkBaseSettings := generateTrunkBaseSettingsResourceWithCustomAttrs(
		phoneTrunkBaseSettingsRes,
		"phone trunk base settings "+uuid.NewString(),
		"",
		"phone_connections_webrtc.json",
		"PHONE",
		false)

	trunkBaseSettingsConfig := generateTrunkBaseSettingsResourceWithCustomAttrs(
		trunkBaseSettingsRes,
		"test trunk base settings "+uuid.NewString(),
		"test description 1",
		"phone_connections_webrtc.json",
		"PHONE",
		false)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			// Create the trunk by creating trunk base settings and an edge group and assigning the trunk base settings to the edge group
			{
				Config: generateEdgeGroupResourceWithCustomAttrs(
					edgeGroupRes1,
					"test edge group "+uuid.NewString(),
					"edge group description 1",
					false,
					false,
					generatePhoneTrunkBaseIds("genesyscloud_telephony_providers_edges_trunkbasesettings."+phoneTrunkBaseSettingsRes+".id")) + phoneTrunkBaseSettings + trunkBaseSettingsConfig + generateTrunk(
					trunkRes,
					"genesyscloud_telephony_providers_edges_trunkbasesettings."+trunkBaseSettingsRes+".id",
					"genesyscloud_telephony_providers_edges_edge_group."+edgeGroupRes1+".id"),
				ExpectNonEmptyPlan: true, // Creating the trunk will edit the edge group and cause configuration drift
			},
			// Create a new edge group and assign the trunk base settings to a new edge group to update the trunk
			{
				Config: generateEdgeGroupResourceWithCustomAttrs(
					edgeGroupRes2,
					"test edge group "+uuid.NewString(),
					"edge group description 2",
					false,
					false,
					generatePhoneTrunkBaseIds("genesyscloud_telephony_providers_edges_trunkbasesettings."+phoneTrunkBaseSettingsRes+".id")) + phoneTrunkBaseSettings + trunkBaseSettingsConfig + generateTrunk(
					trunkRes,
					"genesyscloud_telephony_providers_edges_trunkbasesettings."+trunkBaseSettingsRes+".id",
					"genesyscloud_telephony_providers_edges_edge_group."+edgeGroupRes2+".id"),
				ExpectNonEmptyPlan: true, // Creating the trunk will edit the edge group and cause configuration drift
			},
		},
	})
}

func generateTrunk(
	trunkRes,
	trunkBaseSettingsId,
	edgeGroupId string) string {
	return fmt.Sprintf(`resource "genesyscloud_telephony_providers_edges_trunk" "%s" {
		trunk_base_settings_id = %s
		edge_group_id = %s
	}
	`, trunkRes, trunkBaseSettingsId, edgeGroupId)
}
