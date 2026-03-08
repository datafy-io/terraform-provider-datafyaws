# Use minimal Alpine image
FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy the pre-built binary from the host
COPY terraform-provider-datafyaws ./

# Make it executable
RUN chmod +x ./terraform-provider-datafyaws

CMD ["./terraform-provider-datafyaws"]
