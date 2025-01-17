package genesyscloud

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/mypurecloud/platform-client-sdk-go/v56/platformclientv2"
)

func TestAccResourceRoutingEmailDomainSub(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	var (
		domainRes = "routing-domain1"
		domainId  = "terraform" + strconv.Itoa(rand.Intn(1000))
	)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				// Create purecloud subdomain
				Config: generateRoutingEmailDomainResource(
					domainRes,
					domainId,
					trueValue, // Subdomain clear
					nullValue,
				),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("genesyscloud_routing_email_domain."+domainRes, "domain_id", domainId ),
					resource.TestCheckResourceAttr("genesyscloud_routing_email_domain."+domainRes, "subdomain", trueValue),
				),
			},
			{
				// Import/Read
				ResourceName:      "genesyscloud_routing_email_domain." + domainRes,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
		CheckDestroy: testVerifyRoutingEmailDomainDestroyed,
	})
}

func TestAccResourceRoutingEmailDomainCustom(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	var (
		domainRes       = "routing-domain1"
		domainId        = "terraform" + strconv.Itoa(rand.Intn(1000)) + ".com"
		mailFromDomain1 = "test." + domainId
	)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				// Create custom domain
				Config: generateRoutingEmailDomainResource(
					domainRes,
					domainId,
					falseValue, // Subdomain
					nullValue,
				),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("genesyscloud_routing_email_domain."+domainRes, "domain_id", domainId),
					resource.TestCheckResourceAttr("genesyscloud_routing_email_domain."+domainRes, "subdomain", falseValue),
					resource.TestCheckResourceAttr("genesyscloud_routing_email_domain."+domainRes, "mail_from_domain", ""),
				),
			},
			{
				// Update custom domain
				Config: generateRoutingEmailDomainResource(
					domainRes,
					domainId,
					falseValue, // Subdomain
					strconv.Quote(mailFromDomain1),
				),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("genesyscloud_routing_email_domain."+domainRes, "domain_id", domainId),
					resource.TestCheckResourceAttr("genesyscloud_routing_email_domain."+domainRes, "subdomain", falseValue),
					resource.TestCheckResourceAttr("genesyscloud_routing_email_domain."+domainRes, "mail_from_domain", mailFromDomain1),
				),
			},
			{
				// Import/Read
				ResourceName:      "genesyscloud_routing_email_domain." + domainRes,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
		CheckDestroy: testVerifyRoutingEmailDomainDestroyed,
	})
}

func generateRoutingEmailDomainResource(
	resourceID string,
	domainID string,
	subdomain string,
	fromDomain string) string {
	return fmt.Sprintf(`resource "genesyscloud_routing_email_domain" "%s" {
		domain_id = "%s"
		subdomain = %s
        mail_from_domain = %s
	}
	`, resourceID, domainID, subdomain, fromDomain)
}

func testVerifyRoutingEmailDomainDestroyed(state *terraform.State) error {
	routingAPI := platformclientv2.NewRoutingApi()
	for _, rs := range state.RootModule().Resources {
		if rs.Type != "genesyscloud_routing_email_domain" {
			continue
		}

		domain, resp, err := routingAPI.GetRoutingEmailDomain(rs.Primary.ID)
		if domain != nil {
			return fmt.Errorf("Domain (%s) still exists", rs.Primary.ID)
		} else if isStatus404(resp) {
			// Domain not found as expected
			continue
		} else {
			// Unexpected error
			return fmt.Errorf("Unexpected error: %s", err)
		}
	}
	// Success. All Domains destroyed
	return nil
}
