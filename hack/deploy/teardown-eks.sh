#!/bin/bash
set -e

# Agent Exchange - EKS Teardown Script
# This script removes all EKS resources created for Agent Exchange
# WARNING: This is DESTRUCTIVE and will delete all data!

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Configuration
AWS_REGION="${AWS_REGION:-us-east-1}"
AWS_ACCOUNT_ID="${AWS_ACCOUNT_ID:-}"
CLUSTER_NAME="${CLUSTER_NAME:-aex-eks}"
ENVIRONMENT_NAME="${ENVIRONMENT_NAME:-aex}"
DELETE_INFRA=false

usage() {
    echo "Agent Exchange - EKS Teardown"
    echo ""
    echo "Usage: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  all              Delete all EKS resources (default)"
    echo "  k8s              Delete Kubernetes resources only"
    echo "  helm             Uninstall Helm charts only"
    echo "  cluster          Delete EKS cluster and node groups only"
    echo "  stacks           Delete CloudFormation stacks only"
    echo ""
    echo "Options:"
    echo "  --include-infra  Also delete infrastructure stack (VPC, ECR, secrets)"
    echo ""
    echo "Environment variables:"
    echo "  AWS_REGION         AWS region (default: us-east-1)"
    echo "  AWS_ACCOUNT_ID     AWS account ID (auto-detected)"
    echo "  CLUSTER_NAME       EKS cluster name (default: aex-eks)"
    echo "  ENVIRONMENT_NAME   Environment name prefix (default: aex)"
    echo ""
    echo "WARNING: This will permanently delete all EKS resources and data!"
}

check_prerequisites() {
    echo "Checking prerequisites..."

    if [ -z "$AWS_ACCOUNT_ID" ]; then
        AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text 2>/dev/null || true)
        if [ -z "$AWS_ACCOUNT_ID" ]; then
            echo "Error: Could not determine AWS Account ID"
            exit 1
        fi
    fi

    if ! command -v aws &>/dev/null; then
        echo "Error: AWS CLI is not installed"
        exit 1
    fi

    if ! aws sts get-caller-identity &>/dev/null; then
        echo "Error: Not authenticated with AWS"
        exit 1
    fi

    echo "Prerequisites OK"
}

confirm_deletion() {
    echo ""
    echo "================================================================"
    echo "                          WARNING"
    echo "  This will PERMANENTLY DELETE EKS resources"
    echo "  in AWS region: $AWS_REGION"
    echo "  Account: $AWS_ACCOUNT_ID"
    echo "  Cluster: $CLUSTER_NAME"
    if [ "$DELETE_INFRA" = true ]; then
        echo ""
        echo "  ** ALSO DELETING INFRASTRUCTURE (VPC, ECR, Secrets) **"
    fi
    echo ""
    echo "  This action CANNOT be undone!"
    echo "================================================================"
    echo ""
    read -p "Type 'DELETE' to confirm: " confirmation

    if [ "$confirmation" != "DELETE" ]; then
        echo "Aborted."
        exit 1
    fi
}

# ============================================================
# Delete Kubernetes resources
# ============================================================

delete_k8s_resources() {
    echo "Deleting Kubernetes resources..."

    # Try to configure kubectl
    if aws eks describe-cluster --name "$CLUSTER_NAME" --region "$AWS_REGION" &>/dev/null; then
        aws eks update-kubeconfig --name "$CLUSTER_NAME" --region "$AWS_REGION" 2>/dev/null || true

        # Delete the aex namespace (cascading delete of all resources)
        echo "  Deleting namespace 'aex'..."
        kubectl delete namespace aex --ignore-not-found --timeout=120s 2>/dev/null || true

        # Delete ingress resources (to release load balancers)
        echo "  Deleting ingress resources..."
        kubectl delete ingress --all -n aex 2>/dev/null || true

        # Wait for load balancers to be released
        echo "  Waiting for load balancers to be released..."
        sleep 15

        echo "  Kubernetes resources deleted."
    else
        echo "  Cluster not found or not accessible, skipping K8s cleanup."
    fi
}

# ============================================================
# Uninstall Helm charts
# ============================================================

uninstall_helm_charts() {
    echo "Uninstalling Helm charts..."

    if ! aws eks describe-cluster --name "$CLUSTER_NAME" --region "$AWS_REGION" &>/dev/null; then
        echo "  Cluster not found, skipping Helm cleanup."
        return
    fi

    aws eks update-kubeconfig --name "$CLUSTER_NAME" --region "$AWS_REGION" 2>/dev/null || true

    charts=(
        "cluster-autoscaler:kube-system"
        "metrics-server:kube-system"
        "external-secrets:external-secrets"
        "ingress-nginx:ingress-nginx"
        "aws-load-balancer-controller:kube-system"
    )

    for chart_ns in "${charts[@]}"; do
        IFS=':' read -r chart namespace <<< "$chart_ns"
        echo "  Uninstalling $chart from $namespace..."
        helm uninstall "$chart" -n "$namespace" 2>/dev/null || true
    done

    # Clean up namespaces created by add-ons
    echo "  Deleting add-on namespaces..."
    kubectl delete namespace ingress-nginx --ignore-not-found 2>/dev/null || true
    kubectl delete namespace external-secrets --ignore-not-found 2>/dev/null || true

    # Wait for any load balancers to be released
    echo "  Waiting for resources to be released..."
    sleep 20

    echo "  Helm charts uninstalled."
}

# ============================================================
# Delete EKS cluster
# ============================================================

delete_eks_cluster() {
    echo "Deleting EKS cluster..."

    # Check if cluster exists
    if ! aws eks describe-cluster --name "$CLUSTER_NAME" --region "$AWS_REGION" &>/dev/null; then
        echo "  Cluster '$CLUSTER_NAME' not found, skipping."
        return
    fi

    # Delete node groups first
    echo "  Listing node groups..."
    NODE_GROUPS=$(aws eks list-nodegroups --cluster-name "$CLUSTER_NAME" --region "$AWS_REGION" \
        --query 'nodegroups[*]' --output text 2>/dev/null || echo "")

    for ng in $NODE_GROUPS; do
        echo "  Deleting node group: $ng..."
        aws eks delete-nodegroup \
            --cluster-name "$CLUSTER_NAME" \
            --nodegroup-name "$ng" \
            --region "$AWS_REGION" 2>/dev/null || true
    done

    # Wait for node groups to be deleted
    if [ -n "$NODE_GROUPS" ]; then
        echo "  Waiting for node groups to be deleted..."
        for ng in $NODE_GROUPS; do
            aws eks wait nodegroup-deleted \
                --cluster-name "$CLUSTER_NAME" \
                --nodegroup-name "$ng" \
                --region "$AWS_REGION" 2>/dev/null || true
        done
    fi

    # Delete Fargate profiles if any
    FARGATE_PROFILES=$(aws eks list-fargate-profiles --cluster-name "$CLUSTER_NAME" --region "$AWS_REGION" \
        --query 'fargateProfileNames[*]' --output text 2>/dev/null || echo "")

    for fp in $FARGATE_PROFILES; do
        echo "  Deleting Fargate profile: $fp..."
        aws eks delete-fargate-profile \
            --cluster-name "$CLUSTER_NAME" \
            --fargate-profile-name "$fp" \
            --region "$AWS_REGION" 2>/dev/null || true
        aws eks wait fargate-profile-deleted \
            --cluster-name "$CLUSTER_NAME" \
            --fargate-profile-name "$fp" \
            --region "$AWS_REGION" 2>/dev/null || true
    done

    # Delete the cluster
    echo "  Deleting EKS cluster: $CLUSTER_NAME..."
    aws eks delete-cluster \
        --name "$CLUSTER_NAME" \
        --region "$AWS_REGION" 2>/dev/null || true

    echo "  Waiting for cluster deletion..."
    aws eks wait cluster-deleted \
        --name "$CLUSTER_NAME" \
        --region "$AWS_REGION" 2>/dev/null || true

    echo "  EKS cluster deleted."
}

# ============================================================
# Delete CloudFormation stacks
# ============================================================

delete_cloudformation_stacks() {
    echo "Deleting CloudFormation stacks..."

    # Delete EKS cluster stack
    STACK_NAME="${ENVIRONMENT_NAME}-eks-cluster"
    echo "  Checking stack: $STACK_NAME..."
    if aws cloudformation describe-stacks --stack-name "$STACK_NAME" --region "$AWS_REGION" &>/dev/null; then
        echo "  Deleting $STACK_NAME..."
        aws cloudformation delete-stack \
            --stack-name "$STACK_NAME" \
            --region "$AWS_REGION"

        echo "  Waiting for stack deletion..."
        aws cloudformation wait stack-delete-complete \
            --stack-name "$STACK_NAME" \
            --region "$AWS_REGION" 2>/dev/null || {
                echo "  Warning: Stack deletion may not be complete. Check AWS console."
            }
    else
        echo "  Stack $STACK_NAME not found, skipping."
    fi

    # Optionally delete infrastructure stack
    if [ "$DELETE_INFRA" = true ]; then
        # Delete services stack first (ECS)
        SERVICES_STACK="${ENVIRONMENT_NAME}-services"
        if aws cloudformation describe-stacks --stack-name "$SERVICES_STACK" --region "$AWS_REGION" &>/dev/null; then
            echo "  Deleting $SERVICES_STACK..."
            aws cloudformation delete-stack --stack-name "$SERVICES_STACK" --region "$AWS_REGION"
            aws cloudformation wait stack-delete-complete --stack-name "$SERVICES_STACK" --region "$AWS_REGION" 2>/dev/null || true
        fi

        INFRA_STACK="${ENVIRONMENT_NAME}-infrastructure"
        echo "  Checking stack: $INFRA_STACK..."
        if aws cloudformation describe-stacks --stack-name "$INFRA_STACK" --region "$AWS_REGION" &>/dev/null; then
            echo "  Deleting $INFRA_STACK..."
            aws cloudformation delete-stack \
                --stack-name "$INFRA_STACK" \
                --region "$AWS_REGION"

            echo "  Waiting for stack deletion..."
            aws cloudformation wait stack-delete-complete \
                --stack-name "$INFRA_STACK" \
                --region "$AWS_REGION" 2>/dev/null || {
                    echo "  Warning: Infrastructure stack deletion may not be complete."
                    echo "  ECR repositories with images may need manual deletion."
                }
        else
            echo "  Stack $INFRA_STACK not found, skipping."
        fi
    fi

    echo "  CloudFormation stacks deleted."
}

# ============================================================
# Clean up OIDC provider
# ============================================================

cleanup_oidc() {
    echo "Cleaning up OIDC provider..."

    # Find and delete OIDC providers associated with the cluster
    OIDC_PROVIDERS=$(aws iam list-open-id-connect-providers \
        --query 'OpenIDConnectProviderList[*].Arn' \
        --output text 2>/dev/null || echo "")

    for arn in $OIDC_PROVIDERS; do
        # Check if it's related to our EKS cluster
        ISSUER_URL=$(aws iam get-open-id-connect-provider --open-id-connect-provider-arn "$arn" \
            --query 'Url' --output text 2>/dev/null || echo "")
        if echo "$ISSUER_URL" | grep -q "$AWS_REGION.*eks"; then
            echo "  Deleting OIDC provider: $arn"
            aws iam delete-open-id-connect-provider \
                --open-id-connect-provider-arn "$arn" 2>/dev/null || true
        fi
    done

    echo "  OIDC cleanup complete."
}

# ============================================================
# Print summary
# ============================================================

print_summary() {
    echo ""
    echo "========================================"
    echo "  EKS Teardown Complete!"
    echo "========================================"
    echo ""
    echo "Deleted resources:"
    echo "  - Kubernetes namespace 'aex' and all resources"
    echo "  - Helm charts (LB controller, ingress, external-secrets, metrics-server)"
    echo "  - EKS node groups"
    echo "  - EKS cluster: $CLUSTER_NAME"
    echo "  - CloudFormation stack: ${ENVIRONMENT_NAME}-eks-cluster"
    echo "  - OIDC provider"
    if [ "$DELETE_INFRA" = true ]; then
        echo "  - Infrastructure stack (VPC, ECR, Secrets)"
    else
        echo ""
        echo "NOT deleted (shared with ECS):"
        echo "  - Infrastructure stack (VPC, ECR, Secrets)"
        echo "  To also delete infrastructure: $0 --include-infra"
    fi
    echo ""
    echo "Note: Some resources may take a few minutes to fully delete."
    echo "Remove kubeconfig context: kubectl config delete-context $CLUSTER_NAME"
}

# ============================================================
# Main
# ============================================================

# Parse options
for arg in "$@"; do
    case $arg in
        --include-infra)
            DELETE_INFRA=true
            ;;
    esac
done

# Get command (skip --flags)
CMD=""
for arg in "$@"; do
    case $arg in
        --*) ;;
        *)
            if [ -z "$CMD" ]; then
                CMD="$arg"
            fi
            ;;
    esac
done
CMD="${CMD:-all}"

case "$CMD" in
    -h|--help|help)
        usage
        exit 0
        ;;
    k8s)
        check_prerequisites
        confirm_deletion
        delete_k8s_resources
        ;;
    helm)
        check_prerequisites
        confirm_deletion
        uninstall_helm_charts
        ;;
    cluster)
        check_prerequisites
        confirm_deletion
        delete_eks_cluster
        ;;
    stacks)
        check_prerequisites
        confirm_deletion
        delete_cloudformation_stacks
        ;;
    all)
        check_prerequisites
        echo ""
        echo "Region:  $AWS_REGION"
        echo "Account: $AWS_ACCOUNT_ID"
        echo "Cluster: $CLUSTER_NAME"
        echo ""
        confirm_deletion
        echo ""

        delete_k8s_resources
        echo ""
        uninstall_helm_charts
        echo ""
        delete_eks_cluster
        echo ""
        cleanup_oidc
        echo ""
        delete_cloudformation_stacks
        echo ""
        print_summary
        ;;
    *)
        echo "Unknown command: $CMD"
        usage
        exit 1
        ;;
esac
