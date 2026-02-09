#!/bin/bash
set -e

# Agent Exchange - EKS Setup Script
# This script sets up the required AWS EKS resources for Kubernetes deployment
# It installs prerequisites, creates the cluster, and configures add-ons.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Configuration
AWS_REGION="${AWS_REGION:-us-east-1}"
AWS_ACCOUNT_ID="${AWS_ACCOUNT_ID:-}"
CLUSTER_NAME="${CLUSTER_NAME:-aex-eks}"
ENVIRONMENT_NAME="${ENVIRONMENT_NAME:-aex}"
ENVIRONMENT="${ENVIRONMENT:-dev}"
DRY_RUN=false

usage() {
    echo "Agent Exchange - EKS Setup"
    echo ""
    echo "Usage: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  all            Run all setup steps (default)"
    echo "  prerequisites  Install/verify eksctl, kubectl, helm"
    echo "  cluster        Create EKS cluster via CloudFormation"
    echo "  addons         Install Helm add-ons (LB controller, ingress, etc.)"
    echo "  irsa           Set up IRSA for pod-level AWS permissions"
    echo "  autoscaler     Configure cluster autoscaler"
    echo "  validate       Validate configuration without creating resources"
    echo ""
    echo "Options:"
    echo "  --dry-run      Show what would be done without making changes"
    echo ""
    echo "Environment variables:"
    echo "  AWS_REGION         AWS region (default: us-east-1)"
    echo "  AWS_ACCOUNT_ID     AWS account ID (auto-detected)"
    echo "  CLUSTER_NAME       EKS cluster name (default: aex-eks)"
    echo "  ENVIRONMENT_NAME   Environment name prefix (default: aex)"
    echo "  ENVIRONMENT        Environment: dev, staging, production (default: dev)"
    echo ""
    echo "This script will:"
    echo "  1. Install/verify eksctl, kubectl, helm"
    echo "  2. Deploy infrastructure stack (VPC, ECR, secrets)"
    echo "  3. Deploy EKS cluster via CloudFormation"
    echo "  4. Install AWS Load Balancer Controller (Helm)"
    echo "  5. Install Nginx Ingress Controller (Helm)"
    echo "  6. Install External Secrets Operator (Helm)"
    echo "  7. Install metrics-server for HPA"
    echo "  8. Configure cluster autoscaler"
    echo "  9. Set up IRSA for pod-level AWS permissions"
}

# Parse options
parse_options() {
    for arg in "$@"; do
        case $arg in
            --dry-run)
                DRY_RUN=true
                shift
                ;;
        esac
    done
}

# ============================================================
# Prerequisites
# ============================================================

check_prerequisites() {
    echo "============================================="
    echo "  Checking Prerequisites"
    echo "============================================="
    echo ""

    local errors=0

    # Check AWS CLI
    echo -n "Checking AWS CLI... "
    if command -v aws &>/dev/null; then
        aws_version=$(aws --version 2>&1 | head -1)
        echo "OK ($aws_version)"
    else
        echo "MISSING"
        echo "  Install: https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2.html"
        ((errors++))
    fi

    # Check kubectl
    echo -n "Checking kubectl... "
    if command -v kubectl &>/dev/null; then
        kubectl_version=$(kubectl version --client --short 2>/dev/null || kubectl version --client -o json 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin)['clientVersion']['gitVersion'])" 2>/dev/null || echo "unknown")
        echo "OK ($kubectl_version)"
    else
        echo "MISSING"
        echo "  Install: https://kubernetes.io/docs/tasks/tools/"
        ((errors++))
    fi

    # Check helm
    echo -n "Checking helm... "
    if command -v helm &>/dev/null; then
        helm_version=$(helm version --short 2>/dev/null || echo "unknown")
        echo "OK ($helm_version)"
    else
        echo "MISSING"
        echo "  Install: https://helm.sh/docs/intro/install/"
        ((errors++))
    fi

    # Check eksctl (optional but recommended)
    echo -n "Checking eksctl... "
    if command -v eksctl &>/dev/null; then
        eksctl_version=$(eksctl version 2>/dev/null || echo "unknown")
        echo "OK ($eksctl_version)"
    else
        echo "NOT INSTALLED (optional)"
        echo "  Install: https://eksctl.io/installation/"
    fi

    # Check docker
    echo -n "Checking docker... "
    if command -v docker &>/dev/null; then
        echo "OK"
    else
        echo "MISSING"
        echo "  Install: https://docs.docker.com/get-docker/"
        ((errors++))
    fi

    # Check AWS authentication
    echo ""
    echo -n "Checking AWS authentication... "
    if aws sts get-caller-identity &>/dev/null; then
        identity=$(aws sts get-caller-identity --query 'Arn' --output text)
        echo "OK"
        echo "  Identity: $identity"

        if [ -z "$AWS_ACCOUNT_ID" ]; then
            AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text 2>/dev/null || echo "")
        fi
    else
        echo "FAILED"
        echo "  Not authenticated. Run 'aws configure' or set AWS credentials"
        ((errors++))
    fi

    echo ""
    if [ $errors -gt 0 ]; then
        echo "PREREQUISITE CHECK FAILED: $errors error(s)"
        echo "Please install the missing tools and try again."
        return 1
    else
        echo "All prerequisites satisfied."
        return 0
    fi
}

install_prerequisites() {
    echo "============================================="
    echo "  Installing Prerequisites"
    echo "============================================="
    echo ""

    local os_type
    os_type=$(uname -s | tr '[:upper:]' '[:lower:]')

    # Install kubectl if missing
    if ! command -v kubectl &>/dev/null; then
        echo "Installing kubectl..."
        if [ "$os_type" = "darwin" ]; then
            if command -v brew &>/dev/null; then
                brew install kubectl
            else
                curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/darwin/amd64/kubectl"
                chmod +x kubectl && sudo mv kubectl /usr/local/bin/
            fi
        elif [ "$os_type" = "linux" ]; then
            curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
            chmod +x kubectl && sudo mv kubectl /usr/local/bin/
        fi
        echo "kubectl installed."
    fi

    # Install helm if missing
    if ! command -v helm &>/dev/null; then
        echo "Installing helm..."
        if [ "$os_type" = "darwin" ]; then
            if command -v brew &>/dev/null; then
                brew install helm
            else
                curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
            fi
        elif [ "$os_type" = "linux" ]; then
            curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
        fi
        echo "helm installed."
    fi

    # Install eksctl if missing
    if ! command -v eksctl &>/dev/null; then
        echo "Installing eksctl..."
        if [ "$os_type" = "darwin" ]; then
            if command -v brew &>/dev/null; then
                brew tap weaveworks/tap
                brew install weaveworks/tap/eksctl
            else
                ARCH=$(uname -m)
                curl -sLO "https://github.com/eksctl-io/eksctl/releases/latest/download/eksctl_Darwin_${ARCH}.tar.gz"
                tar -xzf "eksctl_Darwin_${ARCH}.tar.gz" -C /tmp && sudo mv /tmp/eksctl /usr/local/bin
                rm -f "eksctl_Darwin_${ARCH}.tar.gz"
            fi
        elif [ "$os_type" = "linux" ]; then
            ARCH=amd64
            curl -sLO "https://github.com/eksctl-io/eksctl/releases/latest/download/eksctl_Linux_${ARCH}.tar.gz"
            tar -xzf "eksctl_Linux_${ARCH}.tar.gz" -C /tmp && sudo mv /tmp/eksctl /usr/local/bin
            rm -f "eksctl_Linux_${ARCH}.tar.gz"
        fi
        echo "eksctl installed."
    fi

    echo ""
    echo "Prerequisites installation complete."
}

# ============================================================
# Cluster creation
# ============================================================

create_cluster() {
    echo "============================================="
    echo "  Creating EKS Cluster"
    echo "============================================="
    echo ""
    echo "Cluster:     $CLUSTER_NAME"
    echo "Region:      $AWS_REGION"
    echo "Environment: $ENVIRONMENT"
    echo ""

    # Deploy infrastructure stack first
    echo "Deploying infrastructure stack (VPC, ECR, Secrets)..."
    aws cloudformation deploy \
        --template-file "$PROJECT_ROOT/deploy/aws/infrastructure.yaml" \
        --stack-name "${ENVIRONMENT_NAME}-infrastructure" \
        --parameter-overrides EnvironmentName="$ENVIRONMENT_NAME" \
        --capabilities CAPABILITY_NAMED_IAM \
        --region "$AWS_REGION" \
        --no-fail-on-empty-changeset

    echo "Infrastructure stack deployed."
    echo ""

    # Deploy EKS cluster stack
    echo "Deploying EKS cluster stack..."

    case "$ENVIRONMENT" in
        production)
            NODE_TYPE="m5.large"; MIN=3; MAX=10; DESIRED=3 ;;
        staging)
            NODE_TYPE="t3.large"; MIN=2; MAX=5; DESIRED=2 ;;
        *)
            NODE_TYPE="t3.medium"; MIN=2; MAX=5; DESIRED=2 ;;
    esac

    aws cloudformation deploy \
        --template-file "$PROJECT_ROOT/deploy/aws/eks-cluster.yaml" \
        --stack-name "${ENVIRONMENT_NAME}-eks-cluster" \
        --parameter-overrides \
            EnvironmentName="$ENVIRONMENT_NAME" \
            Environment="$ENVIRONMENT" \
            ClusterName="$CLUSTER_NAME" \
            NodeInstanceType="$NODE_TYPE" \
            MinSize="$MIN" \
            MaxSize="$MAX" \
            DesiredSize="$DESIRED" \
        --capabilities CAPABILITY_NAMED_IAM \
        --region "$AWS_REGION" \
        --no-fail-on-empty-changeset

    echo "EKS cluster stack deployed."
    echo ""

    # Configure kubeconfig
    echo "Configuring kubeconfig..."
    aws eks update-kubeconfig \
        --name "$CLUSTER_NAME" \
        --region "$AWS_REGION" \
        --alias "$CLUSTER_NAME"

    echo ""
    echo "EKS cluster is ready."
    kubectl cluster-info
    echo ""
    kubectl get nodes
}

# ============================================================
# Helm add-ons
# ============================================================

install_addons() {
    echo "============================================="
    echo "  Installing Helm Add-ons"
    echo "============================================="
    echo ""

    # Ensure kubeconfig is set
    aws eks update-kubeconfig --name "$CLUSTER_NAME" --region "$AWS_REGION" 2>/dev/null || true

    # Get VPC ID and LB controller role ARN
    VPC_ID=$(aws cloudformation describe-stacks \
        --stack-name "${ENVIRONMENT_NAME}-infrastructure" \
        --query "Stacks[0].Outputs[?OutputKey=='VPCId'].OutputValue" \
        --output text --region "$AWS_REGION")

    LB_ROLE_ARN=$(aws cloudformation describe-stacks \
        --stack-name "${ENVIRONMENT_NAME}-eks-cluster" \
        --query "Stacks[0].Outputs[?OutputKey=='AWSLoadBalancerControllerRoleArn'].OutputValue" \
        --output text --region "$AWS_REGION")

    # --- AWS Load Balancer Controller ---
    echo "1. Installing AWS Load Balancer Controller..."
    helm repo add eks https://aws.github.io/eks-charts 2>/dev/null || true
    helm repo update eks
    helm upgrade --install aws-load-balancer-controller eks/aws-load-balancer-controller \
        --namespace kube-system \
        --set clusterName="$CLUSTER_NAME" \
        --set serviceAccount.create=true \
        --set serviceAccount.name=aws-load-balancer-controller \
        --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="$LB_ROLE_ARN" \
        --set region="$AWS_REGION" \
        --set vpcId="$VPC_ID" \
        --wait
    echo "   AWS Load Balancer Controller installed."
    echo ""

    # --- Nginx Ingress Controller ---
    echo "2. Installing Nginx Ingress Controller..."
    helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx 2>/dev/null || true
    helm repo update ingress-nginx
    kubectl create namespace ingress-nginx 2>/dev/null || true
    helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx \
        --namespace ingress-nginx \
        --set controller.service.type=LoadBalancer \
        --set controller.service.annotations."service\.beta\.kubernetes\.io/aws-load-balancer-type"=nlb \
        --set controller.service.annotations."service\.beta\.kubernetes\.io/aws-load-balancer-scheme"=internet-facing \
        --set controller.metrics.enabled=true \
        --wait
    echo "   Nginx Ingress Controller installed."
    echo ""

    # --- External Secrets Operator ---
    echo "3. Installing External Secrets Operator..."
    helm repo add external-secrets https://charts.external-secrets.io 2>/dev/null || true
    helm repo update external-secrets
    kubectl create namespace external-secrets 2>/dev/null || true
    helm upgrade --install external-secrets external-secrets/external-secrets \
        --namespace external-secrets \
        --set installCRDs=true \
        --wait
    echo "   External Secrets Operator installed."
    echo ""

    # --- Metrics Server ---
    echo "4. Installing Metrics Server (for HPA)..."
    helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/ 2>/dev/null || true
    helm repo update metrics-server
    helm upgrade --install metrics-server metrics-server/metrics-server \
        --namespace kube-system \
        --set args[0]="--kubelet-preferred-address-types=InternalIP" \
        --wait
    echo "   Metrics Server installed."
    echo ""

    echo "All Helm add-ons installed successfully."
}

# ============================================================
# Cluster Autoscaler
# ============================================================

configure_autoscaler() {
    echo "============================================="
    echo "  Configuring Cluster Autoscaler"
    echo "============================================="
    echo ""

    # Ensure kubeconfig is set
    aws eks update-kubeconfig --name "$CLUSTER_NAME" --region "$AWS_REGION" 2>/dev/null || true

    # Install cluster autoscaler via Helm
    echo "Installing Cluster Autoscaler..."
    helm repo add autoscaler https://kubernetes.github.io/autoscaler 2>/dev/null || true
    helm repo update autoscaler
    helm upgrade --install cluster-autoscaler autoscaler/cluster-autoscaler \
        --namespace kube-system \
        --set autoDiscovery.clusterName="$CLUSTER_NAME" \
        --set awsRegion="$AWS_REGION" \
        --set rbac.serviceAccount.create=true \
        --set rbac.serviceAccount.name=cluster-autoscaler \
        --set extraArgs.balance-similar-node-groups=true \
        --set extraArgs.skip-nodes-with-system-pods=false \
        --set extraArgs.expander=least-waste \
        --set extraArgs.scale-down-delay-after-add=5m \
        --set extraArgs.scale-down-unneeded-time=5m \
        --wait

    echo "Cluster Autoscaler configured."
}

# ============================================================
# IRSA Setup
# ============================================================

setup_irsa() {
    echo "============================================="
    echo "  Setting up IRSA (IAM Roles for Service Accounts)"
    echo "============================================="
    echo ""

    # Ensure kubeconfig is set
    aws eks update-kubeconfig --name "$CLUSTER_NAME" --region "$AWS_REGION" 2>/dev/null || true

    POD_ROLE_ARN=$(aws cloudformation describe-stacks \
        --stack-name "${ENVIRONMENT_NAME}-eks-cluster" \
        --query "Stacks[0].Outputs[?OutputKey=='EKSPodRoleArn'].OutputValue" \
        --output text --region "$AWS_REGION")

    echo "Pod Role ARN: $POD_ROLE_ARN"
    echo ""

    # Create namespace
    kubectl create namespace aex 2>/dev/null || true

    # Create annotated service account
    kubectl apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: aex-service-account
  namespace: aex
  annotations:
    eks.amazonaws.com/role-arn: "$POD_ROLE_ARN"
  labels:
    app.kubernetes.io/part-of: agent-exchange
EOF

    echo ""
    echo "IRSA configured."
    echo "Service account 'aex-service-account' in namespace 'aex' is annotated with:"
    echo "  eks.amazonaws.com/role-arn: $POD_ROLE_ARN"
    echo ""
    echo "Add 'serviceAccountName: aex-service-account' to your pod specs to use IRSA."
}

# ============================================================
# Validation
# ============================================================

validate() {
    echo "============================================="
    echo "  EKS Setup Validation"
    echo "============================================="
    echo ""
    echo "Region:      $AWS_REGION"
    echo "Cluster:     $CLUSTER_NAME"
    echo "Environment: $ENVIRONMENT"
    echo ""

    local warnings=0

    # Check prerequisites
    check_prerequisites || return 1

    echo ""

    # Check existing CloudFormation stacks
    echo "Checking existing resources..."
    echo ""

    echo -n "  Infrastructure stack... "
    if aws cloudformation describe-stacks --stack-name "${ENVIRONMENT_NAME}-infrastructure" --region "$AWS_REGION" &>/dev/null; then
        echo "EXISTS"
    else
        echo "WILL CREATE"
    fi

    echo -n "  EKS cluster stack... "
    if aws cloudformation describe-stacks --stack-name "${ENVIRONMENT_NAME}-eks-cluster" --region "$AWS_REGION" &>/dev/null; then
        echo "EXISTS"
        ((warnings++))
    else
        echo "WILL CREATE"
    fi

    echo -n "  EKS cluster '$CLUSTER_NAME'... "
    if aws eks describe-cluster --name "$CLUSTER_NAME" --region "$AWS_REGION" &>/dev/null; then
        STATUS=$(aws eks describe-cluster --name "$CLUSTER_NAME" --region "$AWS_REGION" --query 'cluster.status' --output text)
        echo "EXISTS (status: $STATUS)"
        ((warnings++))
    else
        echo "WILL CREATE"
    fi

    echo ""
    echo "============================================="
    if [ $warnings -gt 0 ]; then
        echo "VALIDATION PASSED with $warnings warning(s)"
        echo "Some resources already exist and may be updated."
    else
        echo "VALIDATION PASSED"
        echo "Ready to create EKS resources."
    fi
    echo ""

    # Estimate costs
    echo "Estimated costs (approximate):"
    echo "  EKS cluster:        \$0.10/hour"
    echo "  t3.medium nodes x2: \$0.084/hour"
    echo "  NAT Gateway:        \$0.045/hour"
    echo "  Load Balancer:      \$0.025/hour"
    echo "  Total (dev):        ~\$0.254/hour (~\$183/month)"
    echo ""
}

# ============================================================
# Print summary
# ============================================================

print_summary() {
    echo ""
    echo "========================================"
    echo "  EKS Setup Complete!"
    echo "========================================"
    echo ""
    echo "Resources created:"
    echo "  - Infrastructure stack (VPC, ECR, Secrets Manager)"
    echo "  - EKS cluster: $CLUSTER_NAME"
    echo "  - Managed node group with auto-scaling"
    echo "  - AWS Load Balancer Controller"
    echo "  - Nginx Ingress Controller"
    echo "  - External Secrets Operator"
    echo "  - Metrics Server"
    echo "  - Cluster Autoscaler"
    echo "  - IRSA-annotated service account"
    echo ""
    echo "Kubeconfig:"
    echo "  aws eks update-kubeconfig --name $CLUSTER_NAME --region $AWS_REGION"
    echo ""
    echo "Next steps:"
    echo "  1. Deploy services: deploy/aws/deploy-eks.sh --region $AWS_REGION --env $ENVIRONMENT"
    echo "  2. Update secrets in AWS Secrets Manager"
    echo "  3. Verify: kubectl get pods -n aex"
    echo ""
    echo "Teardown:"
    echo "  hack/deploy/teardown-eks.sh"
}

# ============================================================
# Main
# ============================================================

parse_options "$@"

# Remove --dry-run from args for case matching
CMD="${1:-all}"
if [ "$CMD" = "--dry-run" ]; then
    CMD="${2:-all}"
fi

case "$CMD" in
    -h|--help|help)
        usage
        exit 0
        ;;
    validate|--validate)
        validate
        ;;
    prerequisites|prereqs)
        check_prerequisites || install_prerequisites
        ;;
    cluster)
        check_prerequisites || { echo "Fix prerequisites first."; exit 1; }
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] Would create EKS cluster: $CLUSTER_NAME"
        else
            create_cluster
        fi
        ;;
    addons)
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] Would install Helm add-ons"
        else
            install_addons
        fi
        ;;
    autoscaler)
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] Would configure cluster autoscaler"
        else
            configure_autoscaler
        fi
        ;;
    irsa)
        if [ "$DRY_RUN" = true ]; then
            echo "[DRY-RUN] Would set up IRSA"
        else
            setup_irsa
        fi
        ;;
    all)
        check_prerequisites || install_prerequisites

        if [ "$DRY_RUN" = true ]; then
            echo ""
            echo "============================================="
            echo "  DRY-RUN MODE - No changes will be made"
            echo "============================================="
            echo ""
            echo "Region:      $AWS_REGION"
            echo "Account:     $AWS_ACCOUNT_ID"
            echo "Cluster:     $CLUSTER_NAME"
            echo "Environment: $ENVIRONMENT"
            echo ""
            echo "The following resources would be created:"
            echo "  - Infrastructure stack (VPC, ECR, Secrets)"
            echo "  - EKS cluster with Kubernetes 1.29"
            echo "  - Managed node group (t3.medium, 2-5 nodes)"
            echo "  - OIDC provider for IRSA"
            echo "  - AWS Load Balancer Controller (Helm)"
            echo "  - Nginx Ingress Controller (Helm)"
            echo "  - External Secrets Operator (Helm)"
            echo "  - Metrics Server (Helm)"
            echo "  - Cluster Autoscaler (Helm)"
            echo "  - IRSA service account in 'aex' namespace"
            echo ""
            echo "Run without --dry-run to create these resources."
        else
            echo ""
            create_cluster
            echo ""
            install_addons
            echo ""
            configure_autoscaler
            echo ""
            setup_irsa
            echo ""
            print_summary
        fi
        ;;
    *)
        echo "Unknown command: $CMD"
        usage
        exit 1
        ;;
esac
