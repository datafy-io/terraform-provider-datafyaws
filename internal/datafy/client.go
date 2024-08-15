package datafy

import (
	awstypes "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/datafy-io/iac-gateway-lambda/iacgateway"
)

type Client struct {
	config Config

	client *iacgateway.AWSClient
}

func NewDatafyClient(config *Config) *Client {
	return &Client{
		config: *config,
		client: iacgateway.NewAwsClient(url, config.Token),
	}
}

func (c *Client) GetVolume(volumeId string) (*Volume, error) {
	volume, err := c.client.GetVolume(volumeId)
	if err != nil {
		return nil, err
	}

	return &Volume{
		Volume: &awstypes.Volume{
			VolumeId: aws.String(volume.VolumeId),
		},
		IsManaged:     volume.IsManaged,
		IsReplacement: volume.IsReplacement,
	}, nil
}

func (c *Client) DeleteVolume(volumeId string, finalSnapshot bool) error {
	return c.client.DeleteVolume(volumeId, finalSnapshot)
}
