# Native release runner

Read this when the release's `native` job is queued, when provisioning a runner, or when reviewing native-release evidence. This describes the Omarchy/Arch approach exercised for the first release; inspect the current host and runner requirements before reusing it.

## Contract and scope

[release.yml](../../../.github/workflows/release.yml) requires `self-hosted`, `Linux`, `X64`, `omarchy`, and `omai-release`. It validates the exact tagged checkout, runs `scripts/test-qml.py`, and uploads `native-preview`. Running the suite locally is useful preparation but does not satisfy the workflow's native dependency.

Provision only for an authorized release/test operation. Check repository runners first:

```sh
gh api repos/pablousx/omai/actions/runners \
  --jq '{total_count,runners:[.runners[]|{name,status,labels:[.labels[].name]}]}'
```

An existing dedicated test account is preferred when available. A one-job ephemeral runner with an empty home and the host home inaccessible is also supported. Do not attach a general-purpose persistent Actions runner to the user's unsandboxed workstation or route pull requests to it. Repository access tokens and runner registration/removal tokens are temporary credentials; never store or print them in project files, screenshots, logs, or issue bodies.

## Prepare an isolated runner

1. Read the current official [runner release](https://github.com/actions/runner/releases/latest), select the Linux x64 archive, and verify its official checksum/digest before extraction. Use a fresh mode-0700 staging directory outside the repository and the watched plugin directory. Do not reuse a hardcoded runner release or an unverified download.
2. Confirm `bwrap`, Omarchy, Python, Git, required .NET libraries, and a working Wayland socket. In the sandbox, `omarchy version` needs the read-only Pacman database. `/etc/resolv.conf` may point into `/run/systemd/resolve`; bind the resolved directory rather than exposing the entire user runtime directory.
3. Bind only the Wayland socket needed by the fixture panels. Do not bind the user home, D-Bus session, SSH agent, GitHub CLI configuration, provider directories, or the whole `/run/user/UID` directory. Clear inherited environment variables. The shared network permits runner downloads; this is filesystem/process isolation, not a claim of complete isolation from every host service.

Example launch shape, after extracting the verified runner archive into `omai_runner_dir`. These variables are task-specific; do not repurpose the host's `HOME` or `XDG_RUNTIME_DIR` shell variables:

```bash
omai_runner_dir=$(mktemp -d "${TMPDIR:-/tmp}/omai-native-runner.XXXXXXXX")
mkdir -m 700 "$omai_runner_dir/home" "$omai_runner_dir/runtime"
# Extract the verified official runner archive into "$omai_runner_dir" here.

omai_wayland=${WAYLAND_DISPLAY:?A native Wayland session is required}
if [[ $omai_wayland != /* ]]; then
  omai_wayland="${XDG_RUNTIME_DIR:?A native runtime directory is required}/$omai_wayland"
fi
test -S "$omai_wayland"
omai_resolver_dir=$(dirname -- "$(readlink -f /etc/resolv.conf)")

omai_runner_sandbox() {
  bwrap --unshare-all --share-net --die-with-parent \
    --ro-bind /usr /usr \
    --symlink usr/bin /bin --symlink usr/lib /lib --symlink usr/lib /lib64 \
    --ro-bind /etc /etc \
    --ro-bind "$omai_resolver_dir" "$omai_resolver_dir" \
    --ro-bind /var/lib/pacman /var/lib/pacman \
    --ro-bind "$omai_wayland" /run/omai/wayland \
    --bind "$omai_runner_dir" /runner \
    --proc /proc --dev /dev --tmpfs /tmp --dir /home \
    --clearenv --setenv PATH /usr/bin:/bin --setenv LANG C.UTF-8 \
    --setenv HOME /runner/home --setenv XDG_RUNTIME_DIR /runner/runtime \
    --setenv WAYLAND_DISPLAY /run/omai/wayland \
    --setenv QT_QUICK_BACKEND software \
    --chdir /runner "$@"
}

omai_runner_sandbox ./bin/Runner.Listener --version
omai_runner_sandbox omarchy version
```

Read the paths before executing; this example assumes the Omarchy/Arch `/usr` layout and Pacman database. If isolation cannot be established, use a dedicated test host rather than dropping the filesystem boundaries. Do not run the runner outside the sandbox as a fallback.

Before registration, exercise a fixture panel in the same sandbox using a read-only temporary mount of the reviewed source checkout and `python3 scripts/test-qml.py --state healthy`. Remove that source mount for the actual runner job, which must check out and test its own exact tag.

## Register, run, and clean up

Obtain a repository-scoped registration token with `gh api --method POST repos/pablousx/omai/actions/runners/registration-token`, capturing output privately. Invoke the official `config.sh` inside the sandbox with:

- URL `https://github.com/pablousx/omai` and a unique task-specific runner name.
- Labels `omarchy,omai-release` (the runner supplies its platform labels).
- `--unattended --ephemeral --disableupdate --work _work` and the captured registration token.

Pass the token without printing it or interpolating it into logged shell source. When using a subprocess helper, avoid exceptions that reproduce the token-bearing command. `--disableupdate` is appropriate for a freshly downloaded one-job runner, not a substitute for keeping persistent runners current.

Run `omai_runner_sandbox ./run.sh` in a managed session. Confirm it is online, then start the authorized tagged release. Monitor in bounded intervals and keep the user informed. The job must use the release SHA, pass native validation, and upload its fixture artifact.

After the job:

1. Verify the native job conclusion and download/review `native-preview` from that release run.
2. Confirm the ephemeral runner deregistered using the runner-list API. If it never picked up a job or remains registered after cancellation, stop only this task's listener and remove only its runner ID using the official removal flow/API.
3. Remove the task's credential files and staging directory only after the listener exits and deregistration is verified. Never delete an unrelated runner or broad `/tmp` contents.

If a transient failure requires another native job, register a new ephemeral runner for the same reviewed SHA. If source changes, use the new release version/tag policy. A green job is evidence for its exact checkout, not for later edits or an unrelated local installation.
