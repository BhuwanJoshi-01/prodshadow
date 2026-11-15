# Container image with the released, statically linked prodshadow binary.
#
# The build context must hold bin/linux_<arch>/prodshadow for each target
# architecture; the container workflow extracts them from the release
# archives (see RELEASING.md). There is no RUN step, so images for other
# architectures build without emulation.
#
# The image runs the commands that need no database (inspect, plan,
# validate, rules, locks, schema, report, diff) in CI systems without Go or
# Node. verify starts PostgreSQL in its own container and needs the binary
# on a host with Docker or Podman.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

ARG TARGETARCH
COPY --chmod=0755 bin/linux_${TARGETARCH}/prodshadow /usr/local/bin/prodshadow

WORKDIR /work
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/prodshadow"]
CMD ["--help"]
