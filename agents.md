# AI Agent Instructions

## Where to find AI instructions

All AI-specific instructions, skills, and commands for this project live under `./.ai/`. Read the entire directory before starting any task:

```
.ai/skills/   # Step-by-step instructions for recurring tasks (e.g. rebasing, releases)
```

Load and follow any relevant files from `.ai/` when carrying out work in this repository.

---

## About this repository

This is **terraform-provider-datafyaws** — a wrapper on top of [hashicorp/terraform-provider-aws](https://github.com/hashicorp/terraform-provider-aws). It is rebased periodically onto upstream hashicorp tags to pick up new AWS resources while preserving Datafy-specific behaviour.

All Datafy additions live in `internal/datafy/` and in targeted overrides inside `internal/service/ec2/` and `internal/provider/`. Commits that belong to Datafy are prefixed `datafy:` or `Datafy:`.

### Why this wrapper exists — the datafied volume problem

When Datafy runs its **datafy action** on an EBS volume it:

1. Deletes the original (source) volume from AWS.
2. Creates **new Datafy-managed volumes** in its place (the "datafy volumes"), identified by a tag that links them back to the original volume ID.

This creates a fundamental conflict with Terraform: Terraform tracks the original volume ID in state, but that volume no longer exists in AWS. Without this wrapper, Terraform would detect the missing volume and keep trying to recreate it — an infinite recreate loop.

This provider intercepts every EBS operation and routes it through the **Datafy SaaS API** (`internal/datafy.Client`) whenever it detects a managed volume, preventing Terraform from acting on stale state.

---

### How the EBS Volume resource works (`ebs_volume.go`)

**CustomizeDiff — blocking unsupported changes**
Before planning any change to a volume, the provider checks with the Datafy API. If the volume is managed, only `size`, `iops`, and `throughput` are allowed to change. Any other attribute change (type, encryption, AZ, etc.) is rejected with an error at plan time — Terraform never even attempts the apply.

#### Read

- If the volume exists in AWS → normal read.
- If the volume is **not found in AWS** (because it was datafied and deleted):
    - If Datafy reports it as **managed**: return the current Terraform state unchanged, updating only `size`/`iops`/`throughput`/`tags` from the Datafy API and the underlying datafy volumes. Terraform sees no drift.
    - If Datafy reports it as **replaced** (`ReplacedBy` is set, meaning it was un-datafied and a fresh AWS volume was created): update the state ID to the new volume and re-read from AWS, handing control back to Terraform.

#### Create

- Snapshot IDs starting with `dsnap-` are Datafy snapshots. The provider calls `dc.CreateVolumeFromSnapshot` instead of the AWS API, then waits for the resulting datafy volumes to become available.
- Normal `snap-` and non-snapshot creates go straight to AWS.

#### Update

- Managed volume: only `size`, `iops`, `throughput` changes are forwarded to `dc.ModifyVolume` (Datafy API). The provider then polls until the Datafy API confirms the new value.
- Replaced volume: redirects to the new volume ID and retries the update against AWS normally.
- Unmanaged volume: standard AWS `ModifyVolume`.

#### Delete

- Managed volume: collect the IDs of all underlying datafy volumes (and the source, if it still exists) and delete them all. The Datafy API controls snapshots and cleanup on its side.
- Replaced volume: redirects to the new volume ID and deletes it normally.
- Unmanaged volume: standard AWS `DeleteVolume`, with optional `final_snapshot` support.

---

### How the EBS Volume Attachment resource works (`ebs_volume_attachment.go`)

**CustomizeDiff — blocking changes**
Any modification to an existing volume attachment whose volume is managed is rejected at plan time.

#### Create

- If the volume is managed, calls `dc.AttachVolume` (Datafy API) instead of the AWS API. The Datafy service attaches all underlying datafy volumes to the instance. The provider then waits for each of them to reach the attached state in AWS.
- Unmanaged: standard AWS `AttachVolume`.

#### Read

- If the attachment is not found in AWS and the volume is managed: the attachment is under Datafy's control — return the existing state unchanged (no drift).
- If the volume has been replaced: update `volume_id` in state to the new ID and re-read.

#### Delete

- Managed volume: calls `dc.DetachVolume` (Datafy API), which detaches all underlying datafy volumes. The provider waits for every volume to reach the detached state in AWS.
- Replaced volume: redirects to the new volume ID and detaches normally.
- Unmanaged: standard AWS `DetachVolume`, with optional `stop_instance_before_detaching`.

---

### Key concepts

| Term | Meaning |
| --- | --- |
| **Managed volume** | An EBS volume that has been datafied. The original AWS volume is deleted; Datafy tracks it and owns two replacement volumes. |
| **Datafy volumes** | The two AWS EBS volumes created by the datafy action. Found via `DescribeDatafiedVolumesInput` (filters by Datafy tag referencing the original volume ID). |
| **Source volume** | The original volume ID Terraform knows. After datafication it no longer exists in AWS. |
| **HasSource** | Whether the source volume still physically exists in AWS (it may have already been deleted by datafication). |
| **ReplacedBy** | Set when a volume is un-datafied — contains the ID of the new AWS volume that replaces it. Signals Terraform to hand control back to AWS. |
| **dsnap- prefix** | Datafy snapshot IDs (vs AWS `snap-` prefix). Creating a volume from a `dsnap-` snapshot is routed through the Datafy API. |
| **Datafy client** | `internal/datafy.Client` — thin HTTP client to the Datafy SaaS API. Obtained via `meta.(*conns.AWSClient).DatafyClient(ctx)`. |
