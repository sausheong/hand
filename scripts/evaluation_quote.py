"""Conservative text-request quotes from an explicitly reviewed price contract.

No prices are fetched or inferred. A quote does not authorise a request. The
gateway must enforce the selected model/output cap and validate the contract's
context, cache-accounting and fee assumptions against the actual provider.
"""
from fractions import Fraction
import hashlib
import json
import re

from evaluation_budget import identity, integer


def amount(value):
    # Exact decimal USD values only: no binary floats, exponents or NaN.
    if not isinstance(value, str) or not re.fullmatch(r'(?:0|[1-9][0-9]{0,17})(?:\.[0-9]{1,18})?', value):
        raise ValueError('bounded nonnegative decimal price string required')
    return Fraction(value)


def quote_request(contract, provider, model, max_output_tokens):
    fields = {'schema_version', 'provider', 'model', 'source_sha256', 'billing',
              'context_tokens', 'max_output_tokens', 'price_sets', 'max_request_fee_usd'}
    if not isinstance(contract, dict) or set(contract) != fields:
        raise ValueError('complete explicit price contract required')
    if type(contract['schema_version']) is not int or contract['schema_version'] != 1:
        raise ValueError('unsupported price contract schema')
    identity(provider); identity(model)
    if contract['provider'] != provider or contract['model'] != model:
        raise ValueError('request model differs from price contract')
    if contract['billing'] != 'text_tokens_with_partitioned_cache':
        raise ValueError('unsupported billing semantics')
    if not isinstance(contract['source_sha256'], str) or not re.fullmatch('[a-f0-9]{64}', contract['source_sha256']):
        raise ValueError('reviewed pricing/limit source digest required')
    context = integer(contract['context_tokens'], True)
    output_limit = integer(contract['max_output_tokens'], True)
    output = integer(max_output_tokens, True)
    if output > output_limit or output > context:
        raise ValueError('requested output exceeds provider limits')
    prices = contract['price_sets']
    if not isinstance(prices, list) or not 1 <= len(prices) <= 64:
        raise ValueError('explicit applicable price sets required')
    inputs, outputs = [], []
    for rates in prices:
        if not isinstance(rates, dict) or set(rates) != {'input', 'output', 'cache_read', 'cache_write'}:
            raise ValueError('each price set requires every token category')
        inputs.extend(amount(rates[k]) for k in ('input', 'cache_read', 'cache_write'))
        outputs.append(amount(rates['output']))
    # Per-million-token USD * 1000 = billionths of a USD per token.
    # Price the entire input window at the highest input/cache rate across all
    # applicable tiers, and the output cap at the highest output rate.
    exact = (context*max(inputs) + output*max(outputs))*1000
    exact += amount(contract['max_request_fee_usd'])*1_000_000_000
    nanos = (exact.numerator + exact.denominator - 1)//exact.denominator
    integer(nanos)
    frozen = json.dumps(contract, sort_keys=True, separators=(',', ':'), allow_nan=False).encode()
    return dict(provider=provider, model=model, input_bound=context, output_bound=output,
                cost_bound=nanos, currency='USD', cost_unit='billionth',
                contract_sha256=hashlib.sha256(frozen).hexdigest(),
                source_sha256=contract['source_sha256'],
                method='full_context_and_enforced_output_cap_at_maximum_applicable_rates',
                limitation='Conditional on verified provider bounds and complete rates/fees; no authorisation or forwarding.')
