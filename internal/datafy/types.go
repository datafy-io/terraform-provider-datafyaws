package datafy

import (
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type Volume struct {
	*types.Volume

	IsManaged     bool
	IsDatafied    bool
	IsReplacement bool
}

func (v *Volume) UnmarshalJSON(data []byte) error {
	iac := struct {
		VolumeId string `json:"volumeId"`

		IsManaged     bool `json:"isManaged"`
		IsDatafied    bool `json:"isDatafied"`
		IsReplacement bool `json:"isReplacement"`
	}{}
	if err := json.Unmarshal(data, &iac); err != nil {
		return err
	}

	v.Volume = &types.Volume{
		VolumeId: aws.String(iac.VolumeId),
	}
	v.IsManaged = iac.IsManaged
	v.IsDatafied = iac.IsDatafied
	v.IsReplacement = iac.IsReplacement

	return nil
}
