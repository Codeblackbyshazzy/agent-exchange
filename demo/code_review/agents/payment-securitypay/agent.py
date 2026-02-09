"""SecurityPay - Security-focused payment processor with premium rewards on security audits."""

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
class SecurityPayAgent(BasePaymentAgent):
    """
    SecurityPay - Security-Focused Payment Processor

    Characteristics:
    - Base fee: 3.0%
    - Rewards: UP TO 4.0% on security audits (BIG CASHBACK!)
    - Net fee: -1.0% on security audits (you earn money!)
    - Processing: Thorough (7 seconds)
    - Fraud protection: Advanced

    Best for:
    - Security audits and penetration testing
    - Architecture reviews
    - High-value security-sensitive transactions

    Fee breakdown:
    - Security Audit: 3.0% - 4.0% = -1.0% (CASHBACK!)
    - Architecture Review: 3.0% - 3.0% = 0% (FREE!)
    - Code Review: 3.0% - 2.0% = 1.0%
    - Development: 3.0% - 1.0% = 2.0%
    - Other: 3.0% - 1.0% = 2.0%
    """

    # Payment provider characteristics
    base_fee_percent: float = 3.0
    processing_time_seconds: int = 7
    supported_methods: list[str] = field(default_factory=lambda: ["card", "bank_transfer", "aex_balance", "crypto"])
    fraud_protection: str = "advanced"

    # Category rewards - specializes in security
    category_rewards: dict[str, float] = field(default_factory=lambda: {
        "security_audit": 4.0,         # BIG CASHBACK!
        "architecture_review": 3.0,    # Free processing
        "code_review": 2.0,
        "development": 1.0,
        "linting": 0.5,
        "default": 1.0,
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

        logger.info(f"SecurityPay initialized: {self.base_fee_percent}% base fee, UP TO 4% rewards on security audits!")
