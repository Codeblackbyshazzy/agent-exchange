#!/bin/bash
set -e

# Agent Exchange - GKE Teardown Script
# Removes all GKE resources created for Agent Exchange
# WARNING: This is DESTRUCTIVE and will delete all data!
#
# Usage:
#   GCP_PROJECT_ID=my-project ./teardown-gke.sh
#   GCP_PROJECT_ID=my-project ./teardown-gke.sh namespace
#   GCP_PROJECT_ID=my-project ./teardown-gke.sh all

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Configuration
PROJECT="${GCP_PROJECT_ID:-}"
REGION="${GCP_REGION:-us-central1}"
CLUSTER_NAME="${GKE_CLUSTER_NAME:-aex-cluster}"
NAMESPACE="aex"

usage() {
    cat <<EOF
Agent Exchange - GKE Teardown

Usage: $0 [command]

Commands:
  all              Delete everything: namespace, Helm charts, cluster, IAM (default)
  namespace        Delete K8s namespace and resources only
  helm             Uninstall Helm charts only
  cluster          Delete GKE cluster only
  iam              Delete IAM resources only
  images           Delete Artifact Registry images only

Environment variables:
  GCP_PROJECT_ID       Google Cloud project ID (required)
  GCP_REGION           Google Cloud region (default: us-central1)
  GKE_CLUSTER_NAME     GKE cluster name (default: aex-cluster)

WARNING: This will permanently delete resources and data!
EOF
}

check_prerequisites() {
    echo "Checking prerequisites..."

    if [[ -z "$PROJECT" ]]; then
        echo "Error: GCP_PROJECT_ID environment variable is required"
        exit 1
    fi

    if ! command -v gcloud &> /dev/null; then
        echo "Error: gcloud CLI is not installed"
        exit 1
    fi

    if ! gcloud auth print-identity-token &> /dev/null; then
        echo "Error: Not authenticated with gcloud. Run 'gcloud auth login'"
        exit 1
    fi

    echo "Prerequisites OK"
}

confirm_deletion() {
    local scope="$1"

    echo ""
    echo "================================================================"
    echo "  WARNING: DESTRUCTIVE ACTION"
    echo "================================================================"
    echo ""
    echo "  Project:  $PROJECT"
    echo "  Region:   $REGION"
    echo "  Cluster:  $CLUSTER_NAME"
    echo "  Scope:    $scope"
    echo ""
    echo "  This action CANNOT be undone!"
    echo ""
    echo "================================================================"
    echo ""

    read -p "Type 'DELETE' to confirm: " confirmation
    if [[ "$confirmation" != "DELETE" ]]; then
        echo "Aborted."
        exit 1
    fi
}

# ============================================================
# Get cluster credentials (best effort)
# ============================================================

get_credentials() {
    gcloud container clusters get-credentials "$CLUSTER_NAME" \
        --region="$REGION" \
        --project="$PROJECT" 2>/dev/null || true
}

# ============================================================
# Delete K8s namespace and resources
# ============================================================

delete_namespace() {
    echo ""
    echo "Deleting K8s namespace '$NAMESPACE' and all resources..."

    if kubectl get namespace "$NAMESPACE" &> /dev/null; then
        # List what will be deleted
        echo "  Resources in namespace '$NAMESPACE':"
        kubectl get all -n "$NAMESPACE" 2>/dev/null || true
        echo ""

        # Delete the namespace (cascades to all resources)
        kubectl delete namespace "$NAMESPACE" --wait=true --timeout=120s 2>/dev/null || true
        echo "  Namespace '$NAMESPACE' deleted"
    else
        echo "  Namespace '$NAMESPACE' not found (already deleted or cluster unreachable)"
    fi
}

# ============================================================
# Uninstall Helm charts
# ============================================================

delete_helm_charts() {
    echo ""
    echo "Uninstalling Helm charts..."

    local charts=(
        "ingress-nginx:ingress-nginx"
        "cert-manager:cert-manager"
        "external-secrets:external-secrets"
        "metrics-server:kube-system"
    )

    for chart_info in "${charts[@]}"; do
        local chart="${chart_info%%:*}"
        local ns="${chart_info##*:}"

        if helm status "$chart" -n "$ns" &> /dev/null; then
            echo "  Uninstalling $chart from namespace $ns..."
            helm uninstall "$chart" -n "$ns" 2>/dev/null || true
        else
            echo "  $chart not found in namespace $ns (skipping)"
        fi
    done

    # Clean up chart namespaces
    local chart_namespaces=(
        "ingress-nginx"
        "cert-manager"
        "external-secrets"
    )

    for ns in "${chart_namespaces[@]}"; do
        if kubectl get namespace "$ns" &> /dev/null; then
            echo "  Deleting namespace $ns..."
            kubectl delete namespace "$ns" --wait=false 2>/dev/null || true
        fi
    done

    echo "  Helm charts uninstalled"
}

# ============================================================
# Delete GKE cluster
# ============================================================

delete_cluster() {
    echo ""
    echo "Deleting GKE cluster '$CLUSTER_NAME'..."

    if gcloud container clusters describe "$CLUSTER_NAME" \
        --region="$REGION" --project="$PROJECT" &> /dev/null; then

        gcloud container clusters delete "$CLUSTER_NAME" \
            --region="$REGION" \
            --project="$PROJECT" \
            --quiet

        echo "  Cluster deleted"
    else
        echo "  Cluster '$CLUSTER_NAME' not found (already deleted)"
    fi

    # Clean up kubectl context
    local context="gke_${PROJECT}_${REGION}_${CLUSTER_NAME}"
    kubectl config delete-context "$context" 2>/dev/null || true
    kubectl config delete-cluster "$context" 2>/dev/null || true
    echo "  kubectl context cleaned up"
}

# ============================================================
# Clean up IAM bindings
# ============================================================

delete_iam_resources() {
    echo ""
    echo "Cleaning up IAM resources..."

    # GKE Workload service account
    local sa_email="aex-gke@$PROJECT.iam.gserviceaccount.com"

    if gcloud iam service-accounts describe "$sa_email" --project="$PROJECT" &> /dev/null; then
        echo "  Deleting service account: $sa_email"

        # Remove IAM bindings first
        local roles=(
            "roles/secretmanager.secretAccessor"
            "roles/datastore.user"
            "roles/logging.logWriter"
            "roles/cloudtrace.agent"
            "roles/monitoring.metricWriter"
        )

        for role in "${roles[@]}"; do
            gcloud projects remove-iam-policy-binding "$PROJECT" \
                --member="serviceAccount:$sa_email" \
                --role="$role" \
                --quiet 2>/dev/null || true
        done

        # Delete the service account
        gcloud iam service-accounts delete "$sa_email" \
            --project="$PROJECT" \
            --quiet 2>/dev/null || true

        echo "  Service account deleted"
    else
        echo "  Service account '$sa_email' not found (already deleted)"
    fi

    # Remove GKE roles from GitHub Actions SA (but keep the SA itself)
    local gh_sa_email="aex-github-actions@$PROJECT.iam.gserviceaccount.com"
    if gcloud iam service-accounts describe "$gh_sa_email" --project="$PROJECT" &> /dev/null; then
        echo "  Removing GKE roles from GitHub Actions SA..."
        local gke_roles=(
            "roles/container.developer"
            "roles/container.clusterViewer"
        )
        for role in "${gke_roles[@]}"; do
            gcloud projects remove-iam-policy-binding "$PROJECT" \
                --member="serviceAccount:$gh_sa_email" \
                --role="$role" \
                --quiet 2>/dev/null || true
        done
        echo "  GKE roles removed (SA preserved for Cloud Run)"
    fi

    echo "  IAM cleanup complete"
}

# ============================================================
# Delete Artifact Registry images
# ============================================================

delete_images() {
    echo ""
    echo "Deleting Artifact Registry images..."

    if ! gcloud artifacts repositories describe aex \
        --location="$REGION" --project="$PROJECT" &> /dev/null; then
        echo "  Repository 'aex' not found (skipping)"
        return 0
    fi

    echo "  Listing images in Artifact Registry..."
    local images
    images=$(gcloud artifacts docker images list \
        "$REGION-docker.pkg.dev/$PROJECT/aex" \
        --format="value(PACKAGE)" \
        --project="$PROJECT" 2>/dev/null | sort -u || echo "")

    if [[ -z "$images" ]]; then
        echo "  No images found"
        return 0
    fi

    echo "  Found images:"
    echo "$images" | while read -r img; do
        echo "    - $img"
    done
    echo ""

    read -p "  Delete all images? (y/N): " confirm_images
    if [[ "$confirm_images" == "y" || "$confirm_images" == "Y" ]]; then
        echo "$images" | while read -r img; do
            if [[ -n "$img" ]]; then
                echo "  Deleting $img..."
                gcloud artifacts docker images delete "$img" \
                    --project="$PROJECT" \
                    --delete-tags \
                    --quiet 2>/dev/null || true
            fi
        done
        echo "  Images deleted"
    else
        echo "  Image deletion skipped"
    fi
}

# ============================================================
# Summary
# ============================================================

print_summary() {
    echo ""
    echo "================================================================"
    echo "  GKE Teardown Complete"
    echo "================================================================"
    echo ""
    echo "  Deleted resources:"
    echo "    - K8s namespace '$NAMESPACE' and all resources within"
    echo "    - Helm charts (ingress-nginx, cert-manager, external-secrets)"
    echo "    - GKE cluster '$CLUSTER_NAME'"
    echo "    - IAM service account 'aex-gke'"
    echo ""
    echo "  Preserved resources:"
    echo "    - Artifact Registry (shared with Cloud Run)"
    echo "    - GitHub Actions service account (shared with Cloud Run)"
    echo "    - Secret Manager secrets (shared with Cloud Run)"
    echo "    - Workload Identity Pool (shared with Cloud Run)"
    echo ""
    echo "  To also delete shared resources, run:"
    echo "    ./hack/deploy/teardown-gcp.sh"
    echo ""
}

# ============================================================
# Main
# ============================================================

case "${1:-all}" in
    -h|--help|help)
        usage
        exit 0
        ;;
    namespace)
        check_prerequisites
        confirm_deletion "namespace only"
        get_credentials
        delete_namespace
        ;;
    helm)
        check_prerequisites
        confirm_deletion "Helm charts only"
        get_credentials
        delete_helm_charts
        ;;
    cluster)
        check_prerequisites
        confirm_deletion "GKE cluster only"
        delete_cluster
        ;;
    iam)
        check_prerequisites
        confirm_deletion "IAM resources only"
        delete_iam_resources
        ;;
    images)
        check_prerequisites
        confirm_deletion "Artifact Registry images"
        delete_images
        ;;
    all)
        check_prerequisites
        echo ""
        echo "Project:  $PROJECT"
        echo "Region:   $REGION"
        echo "Cluster:  $CLUSTER_NAME"
        echo ""
        confirm_deletion "ALL GKE resources"
        echo ""

        get_credentials

        delete_namespace
        echo ""
        delete_helm_charts
        echo ""
        delete_cluster
        echo ""
        delete_iam_resources
        echo ""

        read -p "Also delete Artifact Registry images? (y/N): " del_images
        if [[ "$del_images" == "y" || "$del_images" == "Y" ]]; then
            delete_images
            echo ""
        fi

        print_summary
        ;;
    *)
        echo "Unknown command: $1"
        usage
        exit 1
        ;;
esac
