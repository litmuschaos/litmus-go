FROM golang:1.13.4 as builder

WORKDIR /tmp/litmus/

# Copying the experiments and chaos libraries
COPY . .

# After copying the files, we need to ensure the files beling to the user,
# otherwise we will not be able to write build files
USER root

RUN chown -R 500 /tmp/litmus

USER 500

WORKDIR /tmp/litmus/experiments/generic/

# We need to ensure a reasonable build cache dir, as user 500 does not exist on certain systems,
# and will not have permission to write to /.cache, as the user does not exist
ENV XDG_CACHE_HOME=/tmp/.cache

# Building the executables and placing them in a separate directory
RUN go build -o /tmp/litmus/build/ -mod vendor ./...

# Using as main image the crictl image with copying only the binaries
FROM litmuschaos/crictl:latest 

WORKDIR /tmp/litmus/

COPY --from=builder /tmp/litmus/build .