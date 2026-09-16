package datafy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type MockClient struct {
	mu        sync.RWMutex
	ec2Client *ec2.Client

	volumes         map[string]*Volume
	restoredVolumes map[string]*RestoredVolume
}

func NewMockClient(ec2Client *ec2.Client) *MockClient {
	return &MockClient{ec2Client: ec2Client,
		volumes:         make(map[string]*Volume),
		restoredVolumes: make(map[string]*RestoredVolume),
	}
}

func (m *MockClient) SetVolume(volumeId string, volume *Volume) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.volumes[volumeId] = volume
}

func (m *MockClient) GetVolume(volumeId string) (*Volume, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if volume, exists := m.volumes[volumeId]; exists {
		return volume, nil
	}

	return nil, NotFoundError
}

func (m *MockClient) SetRestoredVolume(dsnapId string, restoredVolume *RestoredVolume) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restoredVolumes[dsnapId] = restoredVolume
}

func (m *MockClient) CreateVolumeFromSnapshot(req CreateVolumeFromSnapshotRequest) (*RestoredVolume, error) {
	m.mu.RLock()
	restoredVolume, exists := m.restoredVolumes[req.DatafySnapshotId]
	m.mu.RUnlock()
	if !exists {
		return nil, NotFoundError
	}

	var tags []types.Tag
	for key, value := range req.Tags {
		tags = append(tags, types.Tag{Key: aws.String(key), Value: aws.String(value)})
	}

	volume := &types.Volume{
		VolumeId:         aws.String(restoredVolume.VolumeId),
		AvailabilityZone: aws.String(req.AvailabilityZone),
		Iops:             aws.Int32(req.Iops),
		Throughput:       aws.Int32(req.Throughput),
		Size:             aws.Int32(restoredVolume.VolumeSizeGB),
		Tags:             tags,
	}

	for range 2 {
		if _, err := m.ec2Client.CreateVolume(context.Background(), &ec2.CreateVolumeInput{
			AvailabilityZone: volume.AvailabilityZone,
			Size:             aws.Int32(10),
			VolumeType:       types.VolumeTypeGp2,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeVolume,
					Tags: append([]types.Tag{
						{
							Key:   aws.String("Managed-By"),
							Value: aws.String("Datafy.io"),
						},
						{
							Key:   aws.String("datafy:source-volume:id"),
							Value: volume.VolumeId,
						},
					}, tags...),
				},
			},
		}); err != nil {
			return nil, err
		}
	}

	m.SetVolume(restoredVolume.VolumeId, &Volume{
		Volume:     volume,
		IsManaged:  true,
		IsDatafied: true,
		HasSource:  false,
	})

	return restoredVolume, nil
}

func (m *MockClient) CreateDatafiedVolume(req CreateVolumeRequest) (*Volume, error) {
	// AWS rejects synthetic vol-ids in DescribeVolumes with
	// InvalidParameterValue, not InvalidVolume.NotFound. The provider's Read
	// path only treats the latter as "missing → fall back to Datafy". Harvest
	// a real AWS-issued vol-id by creating a tiny volume and immediately
	// deleting it; refresh-after-create then surfaces it as NotFound.
	probe, err := m.ec2Client.CreateVolume(context.Background(), &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(req.AvailabilityZone),
		Size:             aws.Int32(1),
		VolumeType:       types.VolumeTypeGp2,
	})
	if err != nil {
		return nil, err
	}
	sourceVolumeId := aws.ToString(probe.VolumeId)
	if _, err := m.ec2Client.DeleteVolume(context.Background(), &ec2.DeleteVolumeInput{VolumeId: &sourceVolumeId}); err != nil {
		return nil, err
	}

	tags := []types.Tag{
		{Key: aws.String(managedByTagKey), Value: aws.String(managedByTagValue)},
		{Key: aws.String(sourceVolumeTagKey), Value: aws.String(sourceVolumeId)},
		{Key: aws.String("datafy:volume-source"), Value: aws.String("native")},
	}
	for key, value := range req.Tags {
		tags = append(tags, types.Tag{Key: aws.String(key), Value: aws.String(value)})
	}

	// A performance array states its performance as an array total and the members split it,
	// standing in for how the API provisions them. Without an array size every member carries
	// the whole stated number, across the pair a volume has always been built from.
	arraySize := req.PerformanceArraySize
	memberIops, memberThroughput := req.Iops, req.Throughput
	if arraySize > 0 {
		memberIops = aws.Int32(aws.ToInt32(req.Iops) / arraySize)
		memberThroughput = aws.Int32(aws.ToInt32(req.Throughput) / arraySize)
	} else {
		arraySize = 2
	}

	for range arraySize {
		if _, err := m.ec2Client.CreateVolume(context.Background(), &ec2.CreateVolumeInput{
			AvailabilityZone: aws.String(req.AvailabilityZone),
			Size:             aws.Int32(int32(req.DiskSize)),
			VolumeType:       types.VolumeTypeGp3,
			Iops:             memberIops,
			Throughput:       memberThroughput,
			TagSpecifications: []types.TagSpecification{
				{ResourceType: types.ResourceTypeVolume, Tags: tags},
			},
		}); err != nil {
			return nil, err
		}
	}

	source := &Volume{
		Volume: &types.Volume{
			VolumeId:         aws.String(sourceVolumeId),
			AvailabilityZone: aws.String(req.AvailabilityZone),
			Size:             aws.Int32(int32(req.DiskSize)),
			Iops:             req.Iops,
			Throughput:       req.Throughput,
		},
		IsManaged:  true,
		IsDatafied: true,
		HasSource:  false,
	}
	m.SetVolume(sourceVolumeId, source)
	return source, nil
}

func (m *MockClient) AttachVolume(instanceId string, volumeId string, _ string) error {
	dvo, err := m.ec2Client.DescribeVolumes(context.Background(), DescribeDatafiedVolumesInput(volumeId))
	if err != nil {
		return err
	} else if len(dvo.Volumes) == 0 {
		return fmt.Errorf("datafy volumes not found for source volume %s", volumeId)
	}

	var ids []string
	for _, v := range dvo.Volumes {
		ids = append(ids, aws.ToString(v.VolumeId))
	}

	for _, id := range ids {
		err := func() error {
			// /dev/sdh is used in the tests
			for _, dn := range []string{"/dev/sda", "/dev/sdb", "/dev/sdc", "/dev/sdd"} {
				if _, err := m.ec2Client.AttachVolume(context.Background(), &ec2.AttachVolumeInput{
					Device:     &dn,
					InstanceId: &instanceId,
					VolumeId:   &id,
				}); err == nil {
					return nil
				}
			}
			return fmt.Errorf("attach volume %s failed", volumeId)
		}()
		if err != nil {
			return err
		}
	}

	return ec2.NewVolumeInUseWaiter(m.ec2Client).Wait(context.Background(), &ec2.DescribeVolumesInput{
		VolumeIds: ids,
	}, time.Minute)
}

func (m *MockClient) DetachVolume(instanceId string, volumeId string) error {
	dvo, err := m.ec2Client.DescribeVolumes(context.Background(), DescribeDatafiedVolumesInput(volumeId))
	if err != nil {
		return err
	} else if len(dvo.Volumes) == 0 {
		return fmt.Errorf("datafy volumes not found for source volume %s", volumeId)
	}

	var ids []string
	for _, v := range dvo.Volumes {
		ids = append(ids, aws.ToString(v.VolumeId))
	}

	for _, id := range ids {
		if _, err := m.ec2Client.DetachVolume(context.Background(), &ec2.DetachVolumeInput{
			InstanceId: &instanceId,
			VolumeId:   &id,
			Force:      aws.Bool(true),
		}); err != nil {
			return err
		}
	}

	return ec2.NewVolumeAvailableWaiter(m.ec2Client).Wait(context.Background(), &ec2.DescribeVolumesInput{
		VolumeIds: ids,
	}, time.Minute)
}

func (m *MockClient) ModifyVolume(volumeId string, req ModifyVolumeRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if vol, exists := m.volumes[volumeId]; exists {
		if req.SizeGb != nil {
			vol.Size = req.SizeGb
		}
		if req.Iops != nil {
			vol.Iops = req.Iops
		}
		if req.Throughput != nil {
			vol.Throughput = req.Throughput
		}
	}

	return nil
}
