package datafy

import (
	"encoding/json"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type RestoredVolume struct {
	VolumeId     string `json:"volumeId"`
	VolumeSizeGB int32  `json:"volumeSizeGB"`
}

type DatafiedVolume struct {
	VolumeId        string
	TargetVolumeIds []string
	DiskSize        int64
}

type Volume struct {
	*types.Volume

	HasSource  bool
	IsManaged  bool
	IsDatafied bool
	ReplacedBy string
}

func (v *Volume) UnmarshalJSON(data []byte) error {
	iac := struct {
		VolumeId string `json:"volumeId"`

		HasSource  bool   `json:"hasSource"`
		IsManaged  bool   `json:"isManaged"`
		IsDatafied bool   `json:"isDatafied"`
		ReplacedBy string `json:"replacedBy"`
		SizeBytes  uint64 `json:"sizeBytes"`
		Iops       int32  `json:"iops"`
		Throughput int32  `json:"throughput"`
	}{}
	if err := json.Unmarshal(data, &iac); err != nil {
		return err
	}

	v.Volume = &types.Volume{
		VolumeId: aws.String(iac.VolumeId),
	}
	v.IsManaged = iac.IsManaged
	v.IsDatafied = iac.IsDatafied
	v.ReplacedBy = iac.ReplacedBy
	v.Size = aws.Int32(int32(iac.SizeBytes / 1024 / 1024 / 1024))
	v.Iops = aws.Int32(iac.Iops)
	v.Throughput = aws.Int32(iac.Throughput)

	return nil
}

type Snapshot struct {
	*types.Snapshot

	Region               string
	ReadyToUse           bool
	SizeBytes            int64
	SnapshotCreationTime *time.Time
	DatafySnapshotIds    []string
}

func (v *Snapshot) UnmarshalJSON(data []byte) error {
	iac := struct {
		SnapshotId string `json:"snapshotId"`
		VolumeId   string `json:"volumeId"`
		Region     string `json:"region"`

		ReadyToUse           bool       `json:"readyToUse"`
		SizeBytes            int64      `json:"sizeBytes"`
		SnapshotCreationTime *time.Time `json:"snapshotCreationTime"`
		DatafySnapshotIds    []string   `json:"datafySnapshotIds,omitempty"`
	}{}
	if err := json.Unmarshal(data, &iac); err != nil {
		return err
	}

	v.Snapshot = &types.Snapshot{
		SnapshotId: aws.String(iac.SnapshotId),
		VolumeId:   aws.String(iac.VolumeId),
	}
	v.Region = iac.Region
	v.ReadyToUse = iac.ReadyToUse
	v.SizeBytes = iac.SizeBytes
	v.SnapshotCreationTime = iac.SnapshotCreationTime
	v.DatafySnapshotIds = iac.DatafySnapshotIds

	return nil
}
