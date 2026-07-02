set -e

echo "=== Building Blood Pressure Tracker Docker Image ==="
docker build -t ghcr.io/mitchfen/blood-pressure-tracker:local -f deploy/Dockerfile .

echo "=== Running Blood Pressure Tracker (Interactive Mode) ==="
echo "Access the app at http://localhost:8080"
echo "Press Ctrl+C to stop and remove the container"

docker run -it --rm \
  -p 8080:8080 \
  ghcr.io/mitchfen/blood-pressure-tracker:local
