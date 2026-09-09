import copy
from fractions import Fraction
from pathlib import Path
import tempfile
import unittest

from evaluation_budget import BudgetExceeded, EvaluationBudget
from evaluation_quote import quote_request


class QuoteTests(unittest.TestCase):
    def setUp(self):
        # Synthetic prices only, not a real provider catalogue or paid approval.
        self.contract = dict(schema_version=1, provider='fixture', model='fixture-model',
            source_sha256='a'*64, billing='text_tokens_with_partitioned_cache',
            context_tokens=12, max_output_tokens=4, max_request_fee_usd='0.000000001',
            price_sets=[dict(input='1', output='3', cache_read='0.1', cache_write='1.25'),
                        dict(input='2', output='4', cache_read='0.2', cache_write='2.5')])

    def quote(self, contract=None, output=4):
        return quote_request(self.contract if contract is None else contract, 'fixture', 'fixture-model', output)

    def test_bound_covers_all_small_input_partitions_and_price_sets(self):
        quote = self.quote()
        for prices in self.contract['price_sets']:
            for uncached in range(13):
                for read in range(13-uncached):
                    for write in range(13-uncached-read):
                        for output in range(5):
                            actual = (uncached*Fraction(prices['input'])+read*Fraction(prices['cache_read'])
                                      +write*Fraction(prices['cache_write'])+output*Fraction(prices['output']))*1000+1
                            self.assertGreaterEqual(quote['cost_bound'], actual)

    def test_fractional_nano_cost_rounds_up_not_to_zero(self):
        self.contract.update(context_tokens=1, max_output_tokens=1, max_request_fee_usd='0')
        self.contract['price_sets'] = [dict(input='0.0000001', output='0', cache_read='0', cache_write='0')]
        self.assertEqual(self.quote(output=1)['cost_bound'], 1)

    def test_absent_prices_unknown_billing_and_invalid_values_are_rejected(self):
        for key in self.contract:
            contract = copy.deepcopy(self.contract); del contract[key]
            with self.assertRaises(ValueError): self.quote(contract)
        for value in [None, True, -1, 0.5, 'NaN', 'Infinity', '-1', '1e3']:
            contract = copy.deepcopy(self.contract); contract['price_sets'][0]['cache_write'] = value
            with self.assertRaises(ValueError): self.quote(contract)
        contract = copy.deepcopy(self.contract); contract['billing'] = 'image_units'
        with self.assertRaises(ValueError): self.quote(contract)

    def test_model_output_limit_and_cost_overflow_are_rejected(self):
        with self.assertRaises(ValueError): quote_request(self.contract, 'fixture', 'different', 4)
        for output in [0, True, 5, 1.5]:
            with self.assertRaises(ValueError): self.quote(output=output)
        self.contract['max_request_fee_usd'] = '999999999999999999'
        with self.assertRaises(ValueError): self.quote()

    def test_quote_identity_changes_with_rates_and_source(self):
        before = self.quote()
        self.contract['price_sets'][0]['input'] = '9'
        after = self.quote()
        self.assertNotEqual(before['contract_sha256'], after['contract_sha256'])
        self.assertGreater(after['cost_bound'], before['cost_bound'])
        self.contract['source_sha256'] = 'b'*64
        self.assertNotEqual(after['contract_sha256'], self.quote()['contract_sha256'])

    def test_quote_reservation_and_actual_settlement_integrate(self):
        quote = self.quote()
        with tempfile.TemporaryDirectory() as directory:
            ledger = EvaluationBudget(Path(directory).resolve()/'budget.sqlite', 'c'*64, quote['cost_bound'], clock=lambda:100)
            try:
                ledger.add_run('run', 3, 40, 12, quote['cost_bound'], 60)
                ledger.reserve('run', 'first', quote['input_bound'], quote['output_bound'], quote['cost_bound'])
                with self.assertRaises(BudgetExceeded):
                    ledger.reserve('run', 'second', quote['input_bound'], quote['output_bound'], quote['cost_bound'])
                ledger.settle('first', 1, 1, 4001, 'd'*64)
                remaining = quote['cost_bound']-4001
                self.assertEqual(remaining, quote['cost_bound']-ledger.totals()['cost_nanos'])
                with self.assertRaises(BudgetExceeded):
                    ledger.reserve('run', 'second', quote['input_bound'], quote['output_bound'], quote['cost_bound'])
            finally:
                ledger.close()


if __name__ == '__main__': unittest.main()
