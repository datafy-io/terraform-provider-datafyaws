---
page_title: "aws_volume_attachment Resource - datafyaws"
description: |-
  Datafy-specific behavior of aws_volume_attachment.
---

# aws_volume_attachment

Attaches an EBS volume to an EC2 instance, with Datafy support. Set `provider = datafyaws` on the resource.

This page describes only what DatafyAWS adds or changes. For every argument and attribute, see the [hashicorp/aws `aws_volume_attachment` documentation](https://registry.terraform.io/providers/hashicorp/aws/4.67.0/docs/resources/volume_attachment).

## Example usage

```hcl
resource "aws_volume_attachment" "example" {
  provider = datafyaws

  device_name = "/dev/sdh"
  volume_id   = aws_ebs_volume.example.id
  instance_id = aws_instance.example.id
}
```

## Datafy-managed volumes

When the volume is [Datafy-managed](https://registry.terraform.io/providers/datafy-io/datafyaws/latest/docs#datafy-managed-volumes):

- Create asks Datafy to attach the volume. Datafy attaches every Datafy-managed volume behind it to the instance, and Terraform waits until all of them are attached.
- Refresh does not report drift while Datafy manages the volume. When the volume is un-datafied, `volume_id` moves to the new volume ID with a warning.
- Any change fails at plan time.
- Destroy asks Datafy to detach the volume, and Terraform waits until every Datafy-managed volume is detached. `force_detach` and `stop_instance_before_detaching` are not applied.
