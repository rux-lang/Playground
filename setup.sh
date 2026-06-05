#!/bin/bash
# Rux Language Playground — Debian 12 Setup
# Review each section before running.
set -euo pipefail

PREFIX="/opt/rux-playground"
BACKEND_PORT=8080
SERVICE_USER=ruxplay
RUX_REPO="https://github.com/rux-lang/Rux.git"
RUX_BRANCH="main"
GO_SRC="${PREFIX}/src"

echo "=== Rux Playground Setup (Debian) ==="
echo "Target: ${PREFIX}"
echo "Port:   ${BACKEND_PORT}"
echo "User:   ${SERVICE_USER}"
echo

phase1() {
    echo "--- Phase 1: System packages ---"
    sudo apt update
    sudo apt install -f -y 2>/dev/null || true
    sudo apt install -y docker.io golang-go git build-essential ninja-build ufw cmake

    sudo docker version
    go version
    echo "Phase 1 done."
}

phase2() {
    echo "--- Phase 2: Directory layout ---"
    sudo mkdir -p "${PREFIX}"/{bin,src,data/tmp}
    echo "Phase 2 done."
}

phase3() {
    echo "--- Phase 3: Build Rux compiler (inside Fedora container) ---"
    local build_dir
    build_dir=$(mktemp -d /tmp/rux-build-XXXXX)

    # Dockerfile that builds Rux with Fedora's modern toolchain
    cat > "${build_dir}/Dockerfile.build" << 'DOCKEREOF'
FROM fedora:latest
RUN dnf install -y git cmake ninja-build gcc-c++ && dnf clean all
RUN git clone --depth 1 https://github.com/rux-lang/Rux.git /Rux
WORKDIR /Rux
RUN cmake -S . -B build -G Ninja -DCMAKE_BUILD_TYPE=Release && \
    cmake --build build
DOCKEREOF

    sudo docker build -t rux-builder -f "${build_dir}/Dockerfile.build" "${build_dir}"
    sudo docker run --rm -v "${PREFIX}/bin:/out" rux-builder cp /Rux/build/rux /out/rux
    sudo docker rmi rux-builder
    rm -rf "${build_dir}"
    "${PREFIX}/bin/rux" version || true
    echo "Phase 3 done."
}

phase4() {
    echo "--- Phase 4: Build container image ---"
    sudo cp Dockerfile entrypoint.sh "${PREFIX}/bin/rux" "${PREFIX}/data/"
    sudo docker build -t rux-playground-img "${PREFIX}/data"
    echo "Phase 4 done."
}

phase5() {
    echo "--- Phase 5: Build Go backend ---"
    sudo cp src/main.go src/index.html "${GO_SRC}/"
    cd "${GO_SRC}"
    [ ! -f go.mod ] && go mod init rux-playground
    go mod tidy
    go build -o "${PREFIX}/bin/backend" .
    echo "Phase 5 done."
}

phase6() {
    echo "--- Phase 6: Service user and docker setup ---"
    if ! id "${SERVICE_USER}" &>/dev/null; then
        sudo useradd -m -d "/home/${SERVICE_USER}" "${SERVICE_USER}"
        echo "Created user ${SERVICE_USER}"
    fi
    sudo usermod -aG docker "${SERVICE_USER}"
    sudo chown -R "${SERVICE_USER}":"${SERVICE_USER}" "${PREFIX}"
    sudo chmod 755 "${PREFIX}/bin/backend" "${PREFIX}/data/tmp"
    echo "Phase 6 done."
}

phase7() {
    echo "--- Phase 7: Firewall configuration ---"
    sudo ufw allow ssh
    sudo ufw allow "${BACKEND_PORT}/tcp"
    sudo ufw --force enable
    sudo ufw status
    echo "Phase 7 done."
}

phase8() {
    echo "--- Phase 8: Install systemd service ---"
    sed -e "s|__PORT__|${BACKEND_PORT}|g" \
        -e "s|__PREFIX__|${PREFIX}|g" \
        -e "s|__USER__|${SERVICE_USER}|g" \
        rux-playground.service | sudo tee /etc/systemd/system/rux-playground.service > /dev/null

    sudo systemctl daemon-reload
    sudo systemctl enable rux-playground.service
    echo
    echo "=== All done ==="
    echo "The playground is deployed to ${PREFIX}:"
    echo "  ${PREFIX}/bin/rux          — Rux compiler"
    echo "  ${PREFIX}/bin/backend      — Go HTTP server"
    echo "  ${PREFIX}/src/             — Go source + embedded HTML"
    echo "  ${PREFIX}/data/            — Dockerfile + entrypoint.sh"
    echo "  ${PREFIX}/data/tmp/        — temp execution dirs"
    echo
    echo "Start:  sudo systemctl start rux-playground"
    echo "Stop:   sudo systemctl stop rux-playground"
    echo "Logs:   sudo journalctl -u rux-playground -f"
    echo
    echo "After updating source files (e.g. index.html), redeploy:"
    echo "  sudo cp ~/rux-playground-setup/src/index.html ${PREFIX}/src/"
    echo "  cd ${PREFIX}/src && sudo go build -o ${PREFIX}/bin/backend ."
    echo "  sudo chown -R ${SERVICE_USER}:${SERVICE_USER} ${PREFIX}"
    echo "  sudo systemctl restart rux-playground"
    echo "Phase 8 done."
}

echo "Available phases:"
echo "  1  System packages"
echo "  2  Directories"
echo "  3  Build Rux compiler"
echo "  4  Container image"
echo "  5  Go backend"
echo "  6  Service user"
echo "  7  Firewall"
echo "  8  Systemd service"
echo "  9  Upgrade everything (rux + container + backend)"
echo "  all  Everything"

if [ $# -eq 0 ]; then
    echo
    echo "Usage: bash setup.sh <phase>"
    exit 0
fi

case "$1" in
    1) phase1 ;;
    2) phase2 ;;
    3) phase3 ;;
    4) phase4 ;;
    5) phase5 ;;
    6) phase6 ;;
    7) phase7 ;;
    8) phase8 ;;
    9)
        echo "--- Upgrade: Rux compiler ---"
        sudo chown "$(whoami)" -R "${PREFIX}" 2>/dev/null || true
        bash "$0" 3
        echo "--- Upgrade: Container image (caches Std/Linux) ---"
        sudo cp Dockerfile entrypoint.sh "${PREFIX}/bin/rux" "${PREFIX}/data/" 2>/dev/null || true
        sudo docker build --no-cache -t rux-playground-img "${PREFIX}/data"
        echo "--- Upgrade: Go backend ---"
        sudo chown "$(whoami)" -R "${PREFIX}" 2>/dev/null || true
        cp src/main.go src/index.html "${GO_SRC}/" 2>/dev/null || true
        cd "${GO_SRC}" && go build -o "${PREFIX}/bin/backend" . 2>/dev/null || true
        sudo chown -R "${SERVICE_USER}":"${SERVICE_USER}" "${PREFIX}" 2>/dev/null || true
        sudo systemctl restart rux-playground
        echo "=== Upgrade done ==="
        ;;
    all)
        phase1
        sudo systemctl enable --now docker
        phase2
        phase3
        sudo chown "$(whoami)" -R "${PREFIX}"
        phase5
        phase6
        phase4
        phase7
        phase8
        echo "=== Done ==="
        echo "Start: sudo systemctl start rux-playground"
        echo "Test via tunnel: ssh -L 8080:localhost:8080 user@vps"
        echo "  curl -X POST http://localhost:8080/run -H 'Content-Type: application/json' -d '{\"code\":\"func Main() -> int { return 0; }\"}'"
        ;;
    *) echo "Unknown phase: $1"; exit 1 ;;
esac
