package datafy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/terraform-provider-aws/version"
)

const DefaultUrl = "https://iac.datafy.io"

type Client interface {
	GetVolume(volumeId string) (*Volume, error)
	CreateVolumeFromSnapshot(req CreateVolumeFromSnapshotRequest) (*RestoredVolume, error)
	CreateDatafiedVolume(req CreateVolumeRequest) (*Volume, error)
	AttachVolume(instanceId string, volumeId string, deviceName string) error
	DetachVolume(instanceId string, volumeId string) error
	ModifyVolume(volumeId string, req ModifyVolumeRequest) error
}

type CreateVolumeRequest struct {
	AvailabilityZone string
	DiskSize         int64
	Iops             *int32
	Throughput       *int32
	Encrypted        *bool
	KmsKeyId         string
	Autoscaling      bool
	Performance      bool
	PerformanceTier  int32
	Tags             map[string]string
}

type CreateVolumeFromSnapshotRequest struct {
	DatafySnapshotId string
	AvailabilityZone string
	Iops             int32
	Throughput       int32
	Tags             map[string]string
}

type ModifyVolumeRequest struct {
	SizeGb     *int32
	Iops       *int32
	Throughput *int32
}

type tags struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func tagsList(tagz map[string]string) []tags {
	list := make([]tags, 0, len(tagz))
	for k, v := range tagz {
		list = append(list, tags{Key: k, Value: v})
	}

	return list
}

type createFromSnapshotsSource struct {
	DatafySnapshotId string `json:"datafySnapshotId"`
}

type createFromSnapshotsVolumeProperties struct {
	AvailabilityZone string `json:"availabilityZone"`
	DiskSize         int64  `json:"diskSize"`
	VolumeIops       int32  `json:"volumeIops"`
	VolumeThroughput int32  `json:"volumeThroughput"`
	Tags             []tags `json:"tags"`
}

type createFromSnapshotsRequest struct {
	Source           createFromSnapshotsSource           `json:"source"`
	VolumeProperties createFromSnapshotsVolumeProperties `json:"volumeProperties"`
}

type createDatafiedVolumeProperties struct {
	AvailabilityZone string `json:"availabilityZone"`
	DiskSize         int64  `json:"diskSize"`
	VolumeIops       *int32 `json:"volumeIops,omitempty"`
	VolumeThroughput *int32 `json:"volumeThroughput,omitempty"`
	Encrypted        *bool  `json:"encrypted,omitempty"`
	KmsKeyId         string `json:"kmsKeyId,omitempty"`
	Tags             []tags `json:"tags,omitempty"`
	Autoscaling      bool   `json:"autoscaling"`
	Performance      bool   `json:"performance"`
	PerformanceTier  int32  `json:"performanceTier"`
}

type createDatafiedVolumeRequest struct {
	VolumeProperties createDatafiedVolumeProperties `json:"volumeProperties"`
}

type attachVolumeRequest struct {
	InstanceId string `json:"instanceId"`
	DeviceName string `json:"deviceName"`
}

type detachVolumeRequest struct {
	InstanceId string `json:"instanceId"`
	Force      bool   `json:"force"`
}

type modifyVolumeRequest struct {
	VolumeSizeGb     *int32 `json:"volumeSizeGb,omitempty"`
	VolumeIops       *int32 `json:"volumeIops,omitempty"`
	VolumeThroughput *int32 `json:"volumeThroughput,omitempty"`
}

type errorResponse struct {
	Message string `json:"message"`
}

type ClientImpl struct {
	url   string
	token string
}

var _ Client = (*ClientImpl)(nil)

func NewDatafyClient(url string, token string) *ClientImpl {
	return &ClientImpl{url: url, token: token}
}

func toError(response *http.Response) error {
	var errResp errorResponse
	if err := json.NewDecoder(response.Body).Decode(&errResp); err == nil && errResp.Message != "" {
		return fmt.Errorf(errResp.Message)
	}
	return fmt.Errorf(response.Status)
}

func drain(response *http.Response) {
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
}

func (c *ClientImpl) sendRequest(method, endpoint string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, fmt.Sprintf("%s/%s", c.url, endpoint), bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Add("User-Agent", fmt.Sprintf("terraform-provider-datafyaws/%s (datafy.io)", version.ProviderVersion))
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.token))
	client := &http.Client{}
	return client.Do(req)
}

func (c *ClientImpl) GetVolume(volumeId string) (*Volume, error) {
	resp, err := c.sendRequest(http.MethodGet, fmt.Sprintf("api/v1/aws/volumes/%s", volumeId), nil)
	if err != nil {
		return nil, err
	}
	defer drain(resp)

	if resp.StatusCode == http.StatusOK {
		var out Volume
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		return &out, nil
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, NotFoundError
	}

	return nil, fmt.Errorf(resp.Status)
}

func (c *ClientImpl) CreateVolumeFromSnapshot(req CreateVolumeFromSnapshotRequest) (*RestoredVolume, error) {
	request := createFromSnapshotsRequest{
		Source: createFromSnapshotsSource{
			DatafySnapshotId: req.DatafySnapshotId,
		},
		VolumeProperties: createFromSnapshotsVolumeProperties{
			VolumeIops:       req.Iops,
			VolumeThroughput: req.Throughput,
			AvailabilityZone: req.AvailabilityZone,
			Tags:             tagsList(req.Tags),
		},
	}

	resp, err := c.sendRequest(http.MethodPost, "api/v1/aws/volumes/create-from-snapshots", request)
	if err != nil {
		return nil, err
	}
	defer drain(resp)

	if resp.StatusCode == http.StatusOK {
		var out RestoredVolume
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		return &out, nil
	}

	return nil, toError(resp)
}

func (c *ClientImpl) CreateDatafiedVolume(req CreateVolumeRequest) (*Volume, error) {
	request := createDatafiedVolumeRequest{
		VolumeProperties: createDatafiedVolumeProperties{
			AvailabilityZone: req.AvailabilityZone,
			DiskSize:         req.DiskSize,
			VolumeIops:       req.Iops,
			VolumeThroughput: req.Throughput,
			Encrypted:        req.Encrypted,
			KmsKeyId:         req.KmsKeyId,
			Tags:             tagsList(req.Tags),
			Autoscaling:      req.Autoscaling,
			Performance:      req.Performance,
			PerformanceTier:  req.PerformanceTier,
		},
	}

	resp, err := c.sendRequest(http.MethodPost, "api/v1/aws/volumes/create-autoscaling-volume", request)
	if err != nil {
		return nil, err
	}
	defer drain(resp)

	if resp.StatusCode == http.StatusOK {
		var out Volume
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		return &out, nil
	}

	return nil, toError(resp)
}

func (c *ClientImpl) AttachVolume(instanceId string, volumeId string, deviceName string) error {
	request := attachVolumeRequest{
		InstanceId: instanceId,
		DeviceName: deviceName,
	}

	resp, err := c.sendRequest(http.MethodPost, fmt.Sprintf("api/v1/aws/volumes/%s/attach", volumeId), request)
	if err != nil {
		return err
	}
	defer drain(resp)

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	return fmt.Errorf(resp.Status)
}

func (c *ClientImpl) DetachVolume(instanceId string, volumeId string) error {
	request := detachVolumeRequest{
		InstanceId: instanceId,
	}

	resp, err := c.sendRequest(http.MethodPost, fmt.Sprintf("api/v1/aws/volumes/%s/detach", volumeId), request)
	if err != nil {
		return err
	}
	defer drain(resp)

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	return fmt.Errorf(resp.Status)
}

func (c *ClientImpl) ModifyVolume(volumeId string, req ModifyVolumeRequest) error {
	request := modifyVolumeRequest{
		VolumeSizeGb:     req.SizeGb,
		VolumeIops:       req.Iops,
		VolumeThroughput: req.Throughput,
	}

	resp, err := c.sendRequest(http.MethodPost, fmt.Sprintf("api/v1/aws/volumes/%s/modify", volumeId), request)
	if err != nil {
		return err
	}
	defer drain(resp)

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		return nil
	}

	return toError(resp)
}
