"""DevPay - General development payment processor with standard rewards."""

import logging
import os
from dataclasses import dataclass, field
from typing import Optional

import sys
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from common.payment_agent import BasePaymentAgent
from common.config import AgentConfig

logger = logging.getLogger(__name__)


@dataclass
class DevPayAgent(BasePaymentAgent):
    """
    DevPay - General Development Payment Processor

    Characteristics:
    - Base fee: 2.0%
    - Rewards: UP TO 1.5% on development categories
    - Net fee: 0.5% on development work
    - Processing: Fast (3 seconds)
    - Fraud protection: Basic

    Best for:
    - General code reviews
    - Development tasks
    - Small to medium transactions

    Fee breakdown:
    - Code Review: 2.0% - 1.0% = 1.0%
    - Development: 2.0% - 1.5% = 0.5%
    - Security Audit: 2.0% - 0.5% = 1.5%
    - Other: 2.0% - 0.5% = 1.5%
    """

    # Payment provider characteristics
    base_fee_percent: float = 2.0
    processing_time_seconds: int = 3
    supported_methods: list[str] = field(default_factory=lambda: ["card", "aex_balance"])
    fraud_protection: str = "basic"

    # Category rewards - general development focus
    category_rewards: dict[str, float] = field(default_factory=lambda: {
        "code_review": 1.0,
        "development": 1.5,
        "linting": 0.5,
        "security_audit": 0.5,
        "architecture_review": 0.5,
        "default": 0.5,
    })

    def __post_init__(self):
        """Initialize with config-based overrides."""
        super().__post_init__()

        # Load from config if available
        if hasattr(self.config, '_raw_config') and 'payment' in self.config._raw_config:
            payment_cfg = self.config._raw_config['payment']
            self.base_fee_percent = payment_cfg.get('base_fee_percent', self.base_fee_percent)
            self.processing_time_seconds = payment_cfg.get('processing_time_seconds', self.processing_time_seconds)
            self.fraud_protection = payment_cfg.get('fraud_protection', self.fraud_protection)
            if 'supported_methods' in payment_cfg:
                self.supported_methods = payment_cfg['supported_methods']
            if 'rewards' in payment_cfg:
                for category, reward in payment_cfg['rewards'].items():
                    self.category_rewards[category] = reward

        logger.info(f"DevPay initialized: {self.base_fee_percent}% base fee, general dev payment processor")
