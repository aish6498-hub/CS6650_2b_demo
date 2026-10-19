## What this deploys

Originally a starter repo deploying a simple album server to ECS/ECR/Fargate via Terraform. Adapted (HW5 Part III) to deploy the Product API from [HW5 Part II](https://github.com/aish6498-hub/CS6650-assignments/tree/main/hw05/part2) instead:
- `src/main.go` replaced with `src/product-service.go`
- `src/go.mod` / `src/go.sum` replaced with the Product API's own
- `src/Dockerfile` replaced with the Product API's Dockerfile, with `--platform=linux/amd64` added to the `FROM` line (Terraform's Docker provider builds locally, so on an ARM machine this forces the correct architecture for Fargate's x86_64 runtime instead of silently producing a mismatched image)
- `container_port` in `terraform/variables.tf` already defaulted to 8080, matching the Product API, so no change needed there

## Instructions

### Prepare Credentials

Retrieve your temporary credentials from Learner's Lab.
Enter your configuration when prompted:
```bash
aws configure
```
Make sure the region is `us-west-2` (matches `terraform/variables.tf`'s default).

And set your session token:
```bash
aws configure set aws_session_token <YOUR-TEMP-SESSION-TOKEN>
```

Verify it worked:
```bash
aws sts get-caller-identity
```
Expected output: a JSON object with your `UserId`, `Account`, and `Arn` — if this errors instead, the credentials weren't set correctly.

### Apply Infrastructure
```bash
cd terraform
terraform init
terraform plan
```
`init` should end with `Terraform has been successfully initialized!`. `plan` should show `Plan: 8 to add, 0 to change, 0 to destroy` (ECR repo, CloudWatch log group, security group, ECS cluster, task definition, ECS service, and the Docker image build + registry push) — if it shows anything to destroy, stop and check what changed before continuing.

```bash
terraform apply -auto-approve
```
Expected output ends with `Apply complete! Resources: 8 added, 0 changed, 0 destroyed.` and prints `ecs_cluster_name` / `ecs_service_name`. This step builds the Docker image locally and pushes it to ECR as part of the apply, so it takes a couple of minutes, most of it spent on the image push.

Note: the ECS service can be created slightly before the image finishes pushing to ECR (Terraform only requires the ECR repository's URL to exist to create the service, not the image itself), so the very first task launch attempt may fail waiting on an image that isn't there yet. ECS automatically retries failed task launches, so this resolves itself within a minute or two without any action needed — just confirm with the status check below rather than assuming "apply complete" alone means a task is actually running.

### Confirm a task is actually running (not just that `apply` succeeded)
```bash
aws ecs list-tasks --cluster $(terraform output -raw ecs_cluster_name) --service-name $(terraform output -raw ecs_service_name)
```
Expected output: one task ARN in `taskArns`.

```bash
aws ecs describe-tasks \
  --cluster $(terraform output -raw ecs_cluster_name) \
  --tasks <TASK_ID_FROM_ABOVE> \
  --query 'tasks[0].{lastStatus:lastStatus,healthStatus:healthStatus,stoppedReason:stoppedReason}'
```
Expected output: `"lastStatus": "RUNNING"`. `"healthStatus": "UNKNOWN"` is normal, there's no explicit container health check configured in the task definition.

### Send Requests
Make sure you are in the `terraform` folder.

Get the public IP address:
```bash
aws ec2 describe-network-interfaces \
--network-interface-ids $(
    aws ecs describe-tasks \
    --cluster $(terraform output -raw ecs_cluster_name) \
    --tasks $(
        aws ecs list-tasks \
        --cluster $(terraform output -raw ecs_cluster_name) \
        --service-name $(terraform output -raw ecs_service_name) \
        --query 'taskArns[0]' --output text
    ) \
    --query "tasks[0].attachments[0].details[?name=='networkInterfaceId'].value" \
    --output text
) \
--query 'NetworkInterfaces[0].Association.PublicIp' \
--output text
```
Expected output: a single public IP address, e.g. `18.246.0.212`.

Send some requests (replace `<PUBLIC-IP-ADDRESS>` with the output above):
```bash
curl -s -w "\nHTTP %{http_code}\n" http://<PUBLIC-IP-ADDRESS>:8080/products/1
```
Expected output (product 1 is pre-seeded):
```
{"product_id":1,"sku":"ABC-123-XYZ","manufacturer":"Acme Corporation","category_id":456,"weight":1250,"some_other_id":789}
HTTP 200
```

```bash
curl -s -w "\nHTTP %{http_code}\n" http://<PUBLIC-IP-ADDRESS>:8080/products/5
```
Expected output (id 5 was never seeded):
```
{"error":"NOT_FOUND","message":"product not found"}
HTTP 404
```

For every other response code (400, 500, and the full POST flow), see [HW5 Part II's README](https://github.com/aish6498-hub/CS6650-assignments/tree/main/hw05/part2) or import the collection and environments from [`hw05/postman/`](https://github.com/aish6498-hub/CS6650-assignments/tree/main/hw05/postman) (`postman_collection.json` plus the Local/Deployed environment files) and select the "Deployed (AWS)" environment, updating its `base_url` to match the current public IP.

## Clean Up
```bash
terraform destroy -auto-approve
```
Expected output ends with `Destroy complete! Resources: 8 destroyed.` Do this once you're done, since the task keeps running (and billing against the Learner Lab budget) until destroyed.
