package genesyscloud

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/mypurecloud/platform-client-sdk-go/v56/platformclientv2"
)

func getAllWebDeployments(ctx context.Context, clientConfig *platformclientv2.Configuration) (ResourceIDMetaMap, diag.Diagnostics) {
	resources := make(ResourceIDMetaMap)
	webDeploymentsAPI := platformclientv2.NewWebDeploymentsApiWithConfig(clientConfig)

	deployments, _, getErr := webDeploymentsAPI.GetWebdeploymentsDeployments()
	if getErr != nil {
		return nil, diag.Errorf("Failed to get web deployments: %v", getErr)
	}

	for _, deployment := range *deployments.Entities {
		resources[*deployment.Id] = &ResourceMeta{Name: *deployment.Name}
	}

	return resources, nil
}

func resourceWebDeployment() *schema.Resource {
	return &schema.Resource{
		Description: "Genesys Cloud Web Deployment",

		CreateContext: createWithPooledClient(createWebDeployment),
		ReadContext:   readWithPooledClient(readWebDeployment),
		UpdateContext: updateWithPooledClient(updateWebDeployment),
		DeleteContext: deleteWithPooledClient(deleteWebDeployment),
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		SchemaVersion: 1,
		Schema: map[string]*schema.Schema{
			"name": {
				Description: "Deployment name",
				Type:        schema.TypeString,
				Required:    true,
			},
			"description": {
				Description: "Deployment description",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"allow_all_domains": {
				Description: "Whether all domains are allowed or not. allowedDomains must be empty when this is true.",
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
			},
			"allowed_domains": {
				Description: "The list of domains that are approved to use this deployment; the list will be added to CORS headers for ease of web use.",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"flow_id": {
				Description: "A reference to the inboundshortmessage flow used by this deployment.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"status": {
				Description: "The current status of the deployment. Valid values: Pending, Active, Inactive, Error, Deleting.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"config_version": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"configuration": {
				Description: "The published configuration version used by this deployment",
				Type:        schema.TypeList,
				Required:    true,
				MaxItems:    1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:         schema.TypeString,
							Optional:     true,
							Computed:     true,
							ExactlyOneOf: []string{"configuration.0.id", "configuration.0.name"},
							//ValidateFunc: validation.ValidConfigurationId,
						},
						"name": {
							Type:         schema.TypeString,
							Optional:     true,
							Computed:     true,
							ExactlyOneOf: []string{"configuration.0.id", "configuration.0.name"},
							//ValidateFunc: validation.ValidConfigurationName,
						},
						"version": {
							Type:             schema.TypeString,
							Optional:         true,
							DiffSuppressFunc: customDiffSuppressFunc,
						},
					},
				},
			},
		},
	}
}

func customDiffSuppressFunc(k, old, new string, d *schema.ResourceData) bool {
	return false
}

func createWebDeployment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	name := d.Get("name").(string)
	description := d.Get("description").(string)
	allowAllDomains := d.Get("allow_all_domains").(bool)
	allowedDomains := interfaceListToStrings(d.Get("allowed_domains").([]interface{}))

	err := validAllowedDomainsSettings(d)
	if err != nil {
		return diag.Errorf("Failed to create web deployment %s: %s", name, err)
	}

	sdkConfig := meta.(*providerMeta).ClientConfig
	api := platformclientv2.NewWebDeploymentsApiWithConfig(sdkConfig)

	log.Printf("Creating web deployment %s", name)

	configId := d.Get("configuration.0.id").(string)
	configVersion := d.Get("configuration.0.version").(string)

	flow := buildSdkDomainEntityRef(d, "flow_id")

	inputDeployment := platformclientv2.Webdeployment{
		Name: &name,
		Configuration: &platformclientv2.Webdeploymentconfigurationversionentityref{
			Id:      &configId,
			Version: &configVersion,
		},
		AllowAllDomains: &allowAllDomains,
		AllowedDomains:  &allowedDomains,
	}

	if description != "" {
		inputDeployment.Description = &description
	}

	if flow != nil {
		inputDeployment.Flow = flow
	}

	deployment, _, err := api.PostWebdeploymentsDeployments(inputDeployment)
	if err != nil {
		return diag.Errorf("Failed to create web deployment %s: %s", name, err)
	}

	d.SetId(*deployment.Id)

	log.Printf("Created web deployment %s %s", name, *deployment.Id)
	return readWebDeployment(ctx, d, meta)
}

func readWebDeployment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	sdkConfig := meta.(*providerMeta).ClientConfig
	api := platformclientv2.NewWebDeploymentsApiWithConfig(sdkConfig)

	log.Printf("Reading web deployment %s", d.Id())
	return withRetriesForRead(ctx, 30*time.Second, d, func() *resource.RetryError {
		deployment, resp, getErr := api.GetWebdeploymentsDeployment(d.Id())
		if getErr != nil {
			if isStatus404(resp) {
				return resource.RetryableError(fmt.Errorf("Failed to read web deployment %s: %s", d.Id(), getErr))
			}
			return resource.NonRetryableError(fmt.Errorf("Failed to read web deployment %s: %s", d.Id(), getErr))
		}

		d.Set("name", *deployment.Name)
		if deployment.Description != nil {
			d.Set("description", *deployment.Description)
		}
		d.Set("configuration", toHCL(deployment.Configuration))
		d.Set("allow_all_domains", *deployment.AllowAllDomains)
		d.Set("allowed_domains", *deployment.AllowedDomains)
		if deployment.Flow != nil {
			d.Set("flow_id", *deployment.Flow)
		}
		d.Set("status", *deployment.Status)

		log.Printf("Read web deployment %s %s %s", d.Id(), *deployment.Name, d.Get("configuration"))
		return nil
	})
}

func toHCL(configuration *platformclientv2.Webdeploymentconfigurationversionentityref) []interface{} {
	hclConfig := make(map[string]interface{})
	hclConfig["id"] = *configuration.Id
	hclConfig["version"] = *configuration.Version
	return []interface{}{hclConfig}
}

func updateWebDeployment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	name := d.Get("name").(string)
	description := d.Get("description").(string)
	allowAllDomains := d.Get("allow_all_domains").(bool)
	allowedDomains := interfaceListToStrings(d.Get("allowed_domains").([]interface{}))

	err := validAllowedDomainsSettings(d)
	if err != nil {
		return diag.Errorf("Failed to update web deployment %s: %s", name, err)
	}

	sdkConfig := meta.(*providerMeta).ClientConfig
	api := platformclientv2.NewWebDeploymentsApiWithConfig(sdkConfig)

	log.Printf("Updating web deployment %s", name)

	configId := d.Get("configuration.0.id").(string)
	configVersion := d.Get("configuration.0.version").(string)

	flow := buildSdkDomainEntityRef(d, "flow_id")

	inputDeployment := platformclientv2.Webdeployment{
		Name: &name,
		Configuration: &platformclientv2.Webdeploymentconfigurationversionentityref{
			Id:      &configId,
			Version: &configVersion,
		},
		AllowAllDomains: &allowAllDomains,
		AllowedDomains:  &allowedDomains,
	}

	if description != "" {
		inputDeployment.Description = &description
	}

	if flow != nil {
		inputDeployment.Flow = flow
	}

	_, _, err = api.PutWebdeploymentsDeployment(d.Id(), inputDeployment)
	if err != nil {
		return diag.Errorf("Error updating web deployment %s: %s", name, err)
	}

	log.Printf("Finished updating web deployment %s", name)
	time.Sleep(5 * time.Second)
	return readWebDeployment(ctx, d, meta)
}

func deleteWebDeployment(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	name := d.Get("name").(string)

	sdkConfig := meta.(*providerMeta).ClientConfig
	api := platformclientv2.NewWebDeploymentsApiWithConfig(sdkConfig)

	log.Printf("Deleting web deployment %s", name)
	_, err := api.DeleteWebdeploymentsDeployment(d.Id())

	if err != nil {
		return diag.Errorf("Failed to delete web deployment %s: %s", name, err)
	}

	return withRetries(ctx, 30*time.Second, func() *resource.RetryError {
		_, resp, err := api.GetWebdeploymentsDeployment(d.Id())
		if err != nil {
			if isStatus404(resp) {
				log.Printf("Deleted web deployment %s", d.Id())
				return nil
			}
			return resource.NonRetryableError(fmt.Errorf("Error deleting web deployment %s: %s", d.Id(), err))
		}

		return resource.RetryableError(fmt.Errorf("Web deployment %s still exists", d.Id()))
	})
}

func validAllowedDomainsSettings(d *schema.ResourceData) error {
	allowAllDomains := d.Get("allow_all_domains").(bool)
	_, allowedDomainsSet := d.GetOk("allowed_domains")

	if allowAllDomains && allowedDomainsSet {
		return errors.New("Allowed domains cannot be specified when all domains are allowed")
	}

	if !allowAllDomains && !allowedDomainsSet {
		return errors.New("Either allowed domains must be specified or all domains must be allowed")
	}

	return nil
}
