"""CodeAuditPay - Code audit payment specialist with highest rewards on code review work."""

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
class CodeAuditPayAgent(BasePaymentAgent):
    """
    CodeAuditPay - Code Audit Payment Specialist

    Characteristics:
    - Base fee: 2.5%
    - Rewards: UP TO 3.5% on security audits (NET CASHBACK!)
    - Net fee: -0.5% on code reviews (you earn money!)
    - Processing: Standard (5 seconds)
    - Fraud protection: Standard

    Best for:
    - Code reviews and audits
    - Security audits
    - Architecture reviews

    Fee breakdown:
    - Code Review: 2.5% - 3.0% = -0.5% (CASHBACK!)
    - Security Audit: 2.5% - 3.5% = -1.0% (CASHBACK!)
    - Architecture Review: 2.5% - 2.5% = 0% (FREE!)
    - Development: 2.5% - 1.0% = 1.5%
    - Other: 2.5% - 1.0% = 1.5%
    """

    # Payment provider characteristics
    base_fee_percent: float = 2.5
    processing_time_seconds: int = 5
    supported_methods: list[str] = field(default_factory=lambda: ["card", "bank_transfer", "aex_balance"])
    fraud_protection: str = "standard"

    # Category rewards - specializes in code audits
    category_rewards: dict[str, float] = field(default_factory=lambda: {
        "code_review": 3.0,            # CASHBACK territory!
        "security_audit": 3.5,         # BIG CASHBACK!
        "architecture_review": 2.5,    # Free processing
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

        logger.info(f"CodeAuditPay initialized: {self.base_fee_percent}% base fee, UP TO 3.5% rewards on audits!")
