package genesyscloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/mypurecloud/platform-client-sdk-go/v55/platformclientv2"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

func getAllFlows(ctx context.Context, clientConfig *platformclientv2.Configuration) (ResourceIDMetaMap, diag.Diagnostics) {
	resources := make(ResourceIDMetaMap)
	architectAPI := platformclientv2.NewArchitectApiWithConfig(clientConfig)

	for pageNum := 1; ; pageNum++ {
		flows, _, err := architectAPI.GetFlows(nil, pageNum, 25, "", "", nil, "", "", "", "", "", "", "", "", false, true, "", "", nil)
		if err != nil {
			return nil, diag.Errorf("Failed to get page of flows: %v", err)
		}

		if flows.Entities == nil || len(*flows.Entities) == 0 {
			break
		}

		for _, flow := range *flows.Entities {
			resources[*flow.Id] = &ResourceMeta{Name: *flow.Name}
		}
	}

	return resources, nil
}

func architectFlowExporter() *ResourceExporter {
	return &ResourceExporter{
		GetResourcesFunc: getAllWithPooledClient(getAllFlows),
		RefAttrs:         map[string]*RefAttrSettings{},
	}
}

func resourceFlow() *schema.Resource {
	return &schema.Resource{
		Description: "Genesys Cloud Flow",

		CreateContext: createWithPooledClient(createFlow),
		ReadContext:   readWithPooledClient(readFlow),
		UpdateContext: updateWithPooledClient(updateFlow),
		DeleteContext: deleteWithPooledClient(deleteFlow),
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		SchemaVersion: 1,
		Schema: map[string]*schema.Schema{
			"filepath": {
				Description: "YAML file path for flow configuration.",
				Type:        schema.TypeString,
				Required:    true,
				StateFunc: func(v interface{}) string {
					return hashFileContent(v.(string))
				},
			},
		},
	}
}

func createFlow(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	sdkConfig := meta.(*providerMeta).ClientConfig
	architectAPI := platformclientv2.NewArchitectApiWithConfig(sdkConfig)

	apiClient := &architectAPI.Configuration.APIClient
	//path := architectAPI.Configuration.BasePath + "/api/v2/flows/jobs"
	path := "http://localhost:8080/api/v2/flows/jobs"

	headerParams := make(map[string]string)

	// add default headers if any
	for key := range architectAPI.Configuration.DefaultHeader {
		headerParams[key] = architectAPI.Configuration.DefaultHeader[key]
	}

	headerParams["Authorization"] = "Bearer " + architectAPI.Configuration.AccessToken
	headerParams["Content-Type"] = "application/json"
	headerParams["Accept"] = "application/json"

	successPayload := make(map[string]interface{})
	response, err := apiClient.CallAPI(path, "POST", nil, headerParams, nil, nil, "", nil)
	if err != nil {
		// Nothing special to do here, but do avoid processing the response
	} else if err == nil && response.Error != nil {
		return diag.Errorf("Failed to register Archy job. %s", err)
	} else {
		err = json.Unmarshal([]byte(response.RawBody), &successPayload)
		if err != nil {
			return diag.Errorf("Failed to unmarshal response after registering the Archy job. %s", err)
		}
	}

	presignedUrl := successPayload["presignedUrl"].(string)
	jobId := successPayload["jobId"].(string)
	correlationId := response.CorrelationID
	headers := successPayload["headers"].(map[string]interface{})
	orgID := headers["x-amz-meta-organizationid"].(string)

	filePath := d.Get("filepath").(string)

	_, err = prepareAndUploadFile(filePath, headers, presignedUrl, jobId, orgID, correlationId)

	if err != nil {
		return diag.Errorf(err.Error())
	}

	flowID := ""

	//TODO: Test 16 mins
	retryErr := withRetries(ctx, 16*time.Minute, func() *resource.RetryError {
		//body, resp, err := architectAPI.getStatus()
		path :=
		//"http://localhost:8080/api/v2/flows/jobs/3f1d37d6-4f15-4cbd-a8ba-b5e7f38e672b"
			"http://localhost:8080/api/v2/flows/jobs/" + jobId
		res := make(map[string]interface{})
		response, err := apiClient.CallAPI(path, "GET", nil, headerParams, nil, nil, "", nil)
		path = "changed"
		if err != nil {
			// Nothing special to do here, but do avoid processing the response
		} else if err == nil && response.Error != nil {
			resource.NonRetryableError(fmt.Errorf("Error retrieving job status. JobID: %s, error: %s", jobId, response.ErrorMessage))
		} else {
			err = json.Unmarshal([]byte(response.RawBody), &res)
			if err != nil {
				resource.NonRetryableError(fmt.Errorf("Unable to unmarshal response when retrieving job status. JobID: %s, error: %s", jobId, response.ErrorMessage))
			}
		}
		if res["status"] == "Failure" {
			return resource.NonRetryableError(fmt.Errorf("Flow publish failed. JobID: %s, exit code: %f", jobId, res["exitCode"].(float64)))
		}

		if res["status"] == "Success" {
			flowID = res["flow"].(map[string]interface{})["id"].(string)
			return nil
		}

		time.Sleep(15 * time.Second) // Wait 15 seconds for next retry
		return resource.RetryableError(fmt.Errorf("Job (%s) is still in progress.", jobId))
	})

	if retryErr != nil {
		return retryErr
	}

	if flowID == "" {
		return diag.Errorf("The Architect Job (%s) timed out.", jobId)
	}

	d.SetId(flowID)
	log.Printf("Created flow %s. ", d.Id())
	return readFlow(ctx, d, meta)
}

func readFlow(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	sdkConfig := meta.(*providerMeta).ClientConfig
	architectAPI := platformclientv2.NewArchitectApiWithConfig(sdkConfig)

	apiClient := &architectAPI.Configuration.APIClient
	path := architectAPI.Configuration.BasePath + "/api/v2/flows/" + d.Id()

	headerParams := make(map[string]string)

	// add default headers if any
	for key := range architectAPI.Configuration.DefaultHeader {
		headerParams[key] = architectAPI.Configuration.DefaultHeader[key]
	}

	headerParams["Authorization"] = "Bearer " + architectAPI.Configuration.AccessToken
	headerParams["Content-Type"] = "application/json"
	headerParams["Accept"] = "application/json"

	successPayload := make(map[string]interface{})
	response, err := apiClient.CallAPI(path, "GET", nil, headerParams, nil, nil, "", nil)
	if err != nil {
		// Nothing special to do here, but do avoid processing the response
	} else if err == nil && response.Error != nil {
		return diag.Errorf("Failed to register Archy job. %s", err)
	} else {
		err = json.Unmarshal([]byte(response.RawBody), &successPayload)
		if err != nil {
			return diag.Errorf("Failed to unmarshal response after registering the Archy job. %s", err)
		}
	}

	//flow, resp, err := architectAPI.GetFlow(d.Id(), false)
	//if err != nil {
	//	if isStatus404(resp) {
	//		d.SetId("")
	//		return nil
	//	}
	//	return diag.Errorf("Failed to read flow %s: %s", d.Id(), err)
	//}

	//log.Printf("Read flow %s %s", d.Id(), *flow.Name)
	return nil
}

func updateFlow(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	sdkConfig := meta.(*providerMeta).ClientConfig
	architectAPI := platformclientv2.NewArchitectApiWithConfig(sdkConfig)

	apiClient := &architectAPI.Configuration.APIClient
	//path := architectAPI.Configuration.BasePath + "/api/v2/flows/jobs"
	path := "http://localhost:8080/api/v2/flows/jobs"

	headerParams := make(map[string]string)

	// add default headers if any
	for key := range architectAPI.Configuration.DefaultHeader {
		headerParams[key] = architectAPI.Configuration.DefaultHeader[key]
	}

	headerParams["Authorization"] = "Bearer " + architectAPI.Configuration.AccessToken
	headerParams["Content-Type"] = "application/json"
	headerParams["Accept"] = "application/json"

	successPayload := make(map[string]interface{})
	response, err := apiClient.CallAPI(path, "POST", nil, headerParams, nil, nil, "", nil)
	if err != nil {
		// Nothing special to do here, but do avoid processing the response
	} else if err == nil && response.Error != nil {
		return diag.Errorf("Failed to register Archy job. %s", err)
	} else {
		err = json.Unmarshal([]byte(response.RawBody), &successPayload)
		if err != nil {
			return diag.Errorf("Failed to unmarshal response after registering the Archy job. %s", err)
		}
	}

	presignedUrl := successPayload["presignedUrl"].(string)
	jobId := successPayload["jobId"].(string)
	correlationId := response.CorrelationID
	headers := successPayload["headers"].(map[string]interface{})
	orgID := headers["x-amz-meta-organizationid"].(string)

	filePath := d.Get("filepath").(string)

	_, err = prepareAndUploadFile(filePath, headers, presignedUrl, jobId, orgID, correlationId)

	if err != nil {
		return diag.Errorf(err.Error())
	}

	//TODO: Test 16 mins
	retryErr := withRetries(ctx, 16*time.Minute, func() *resource.RetryError {
		//body, resp, err := architectAPI.getStatus()
		path :=
		//"http://localhost:8080/api/v2/flows/jobs/3f1d37d6-4f15-4cbd-a8ba-b5e7f38e672b"
			"http://localhost:8080/api/v2/flows/jobs/" + jobId
		res := make(map[string]interface{})
		response, err := apiClient.CallAPI(path, "GET", nil, headerParams, nil, nil, "", nil)
		if err != nil {
			// Nothing special to do here, but do avoid processing the response
		} else if err == nil && response.Error != nil {
			resource.NonRetryableError(fmt.Errorf("Error retrieving job status. JobID: %s, error: %s", jobId, response.ErrorMessage))
		} else {
			err = json.Unmarshal([]byte(response.RawBody), &res)
			if err != nil {
				resource.NonRetryableError(fmt.Errorf("Unable to unmarshal response when retrieving job status. JobID: %s, error: %s", jobId, response.ErrorMessage))
			}
		}
		if res["status"] == "Failure" {
			return resource.NonRetryableError(fmt.Errorf("Flow publish failed. JobID: %s, exit code: %f", jobId, res["exitCode"].(float64)))
		}

		if res["status"] == "Success" {
			flowID := res["flow"].(map[string]interface{})["id"].(string)
			d.SetId(flowID)
			return nil
		}

		time.Sleep(15 * time.Second) // Wait 15 seconds for next retry
		return resource.RetryableError(fmt.Errorf("Job (%s) is still in progress.", jobId))
	})

	if retryErr != nil {
		return retryErr
	}

	log.Printf("Created flow %s. ", d.Id())
	return readFlow(ctx, d, meta)
}

func deleteFlow(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	sdkConfig := meta.(*providerMeta).ClientConfig
	architectAPI := platformclientv2.NewArchitectApiWithConfig(sdkConfig)

	_, err := architectAPI.DeleteFlow(d.Id())
	if err != nil {
		return diag.Errorf("Failed to delete the flow %s: %s", d.Id(), err)
	}
	log.Printf("Deleted flow %s", d.Id())
	return nil
}

func hashFileContent(path string) string {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		log.Fatal(err)
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func prepareAndUploadFile(filename string, headers map[string]interface{}, presignedUrl string, jobId string, orgId string, correlationId string) ([]byte, error) {

	//file, err := os.Open(filename)
	//
	//if err != nil {
	//	return nil, fmt.Errorf("Failed to open file %s . Error: %s ", filename, err)
	//}
	//
	//defer file.Close()

	bodyBuf := &bytes.Buffer{}
	//bodyWriter := multipart.NewWriter(bodyBuf)
	//
	//fileWriter, err := bodyWriter.CreateFormFile("uploadfile", filename)
	//if err != nil {
	//	return nil, fmt.Errorf("Failed to write file to the buffer. Error: %s ", err)
	//}

	fh, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("Failed to open the file. Error: %s ", err)
	}
	defer fh.Close()

	_, err = io.Copy(bodyBuf, fh)
	if err != nil {
		return nil, fmt.Errorf("Failed to copy file content to the handler. Error: %s ", err)
	}

	//contentType := bodyWriter.FormDataContentType()
	//bodyWriter.Close()

	req, _ := http.NewRequest("PUT", presignedUrl, bodyBuf)
	//req.Header.Set("Accept", "*/*")
	//req.Header.Set("Content-Type", contentType)

	for key, value := range headers {
		req.Header.Set(key, value.(string))
	}

	client := &http.Client{}

	resp, err := client.Do(req)

	defer resp.Body.Close()

	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Failed to upload flow configuration file to S3 bucket. Error: %s ", err)
	}

	response, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed to read response body when uploading flow configuration file. %s", err)
	}

	return response, nil
}
