---
page_title: "aws_ebs_volume Resource - datafyaws"
description: |-
  Datafy-specific behavior of aws_ebs_volume.
---

# aws_ebs_volume

Manages an EBS volume with Datafy support. Set `provider = datafyaws` on the resource.

This page describes only what DatafyAWS adds or changes. For every other argument and attribute, see the [hashicorp/aws `aws_ebs_volume` documentation](https://registry.terraform.io/providers/hashicorp/aws/4.67.0/docs/resources/ebs_volume). For what changes once Datafy manages a volume, see [Datafy-managed volumes](https://registry.terraform.io/providers/datafy-io/datafyaws/latest/docs#datafy-managed-volumes).

## Example usage

### Regular EBS volume

```hcl
resource "aws_ebs_volume" "example" {
  provider = datafyaws

  availability_zone = "us-east-1a"
  type              = "gp3"
  size              = 40

  tags = {
    Name = "example-volume"
  }
}
```

### Datafy volume

```hcl
resource "aws_ebs_volume" "data" {
  provider = datafyaws

  availability_zone       = "us-east-1a"
  type                    = "gp3"
  size                    = 500
  iops                    = 24000 # optional, total for the array
  throughput              = 2000  # optional, total for the array
  datafy_autoscaling      = true
  datafy_performance      = true
  datafy_performance_tier = 4

  tags = {
    Name = "data"
  }
}
```

### Restoring from a Datafy snapshot

```hcl
resource "aws_ebs_volume" "restored" {
  provider = datafyaws

  availability_zone = "us-east-1a"
  snapshot_id       = "dsnap-0123456789abcdef0"
}
```

## Argument reference

The following are **creation parameters**. They take effect only when the volume is created, and create it directly as a Datafy volume instead of as a regular EBS volume that Datafy datafies later. They cannot be added to, changed on, or removed from an existing volume: any such change fails at plan time. To get a different optimization, create a new volume.

- `datafy_autoscaling` (Optional, Boolean, creation only) - Create the volume optimized for capacity: Datafy grows and shrinks the storage behind it.
- `datafy_performance` (Optional, Boolean, creation only) - Create the volume optimized for performance: Datafy backs it with an array of EBS volumes. Requires `datafy_performance_tier`.
- `datafy_performance_tier` (Optional, Number, creation only) - Number of EBS volumes in the performance array: `4`, `6`, or `8`. Valid only with `datafy_performance = true`.

`datafy_autoscaling` and `datafy_performance` can be combined. With neither set, the volume is created as a regular EBS volume.

- `type` must be `"gp3"`.
- With `datafy_performance`, `iops` and `throughput` are what the whole array delivers, not per EBS volume. `throughput` can be up to 1000 MiB/s times `datafy_performance_tier`; without `datafy_performance` the gp3 range of 125-1000 MiB/s applies. The provider does not range-check `iops`; Datafy sizes the array to deliver it.
- Datafy receives `availability_zone`, `size`, `iops`, `throughput`, `encrypted`, `kms_key_id`, `tags`, and the three `datafy_*` arguments. Do not combine them with `snapshot_id`, `multi_attach_enabled`, or `outpost_arn`; those are not applied to a Datafy volume.
- The volume is Datafy-managed from creation.
- The one exception to "cannot be removed": once the volume is no longer datafied, you can delete these arguments from the configuration. This only updates state; the volume itself is not changed.

### Upgrading from `autoscaling_native`

Releases up to and including 4067.4.0 had a single argument, `autoscaling_native`, which `datafy_autoscaling` replaces. When you upgrade the provider, replace `autoscaling_native = true` with `datafy_autoscaling = true` in your configuration; Terraform rejects `autoscaling_native` as an unsupported argument. The provider converts existing state automatically, so the next plan shows no changes.

## Restoring from a Datafy snapshot

To create a volume from a Datafy snapshot, set `snapshot_id` to the Datafy snapshot ID (`dsnap-…`).

- Datafy receives `availability_zone`, `iops`, `throughput`, and `tags`. The size, type, and encryption come from the snapshot. If you set `size` or `type`, set them to the values the restored volume has, otherwise the next plan shows a change that Datafy rejects.
- The restored volume is Datafy-managed.
- A Datafy snapshot is backed by EBS snapshots (`snap-…`) in your account. Using one of those EBS snapshots as `snapshot_id` fails at apply time with an error naming the Datafy snapshot to use instead.

## Tags

- Tag keys that start with `datafy:` are reserved for Datafy. Setting one in `tags` fails at plan time.
- Datafy's own tags on the Datafy-managed volumes (keys starting with `datafy:`, and `Managed-By = Datafy.io`) are not stored in `tags` or `tags_all`, so they never show up as drift.

## final_snapshot

`final_snapshot` is not supported for Datafy-managed volumes. On destroy the provider tries to snapshot the original volume ID, which either fails (the original volume no longer exists) or snapshots the original volume, not the data Datafy manages. Because a managed volume rejects every argument change other than `size`, `iops`, `throughput`, and `tags`, `final_snapshot` cannot be switched off after Datafy has datafied the volume. Leave `final_snapshot` unset on volumes Datafy may manage, and use Datafy snapshots instead.

## Destroy

Destroying a Datafy-managed volume deletes the Datafy-managed volumes (and the original volume, if it still exists).
