-- +goose Up
-- finance.Merchant now drops "DIRECT DEBIT" and a leading foreign amount's
-- "POUND STERLING". Rules saved from the old merchants keyed on those words
-- alone and recategorized every Fidelity debit or charge abroad, so undo what
-- they set (back to review; what the person set by hand stays), drop them,
-- and bring stored merchants in line with what an import computes now.
UPDATE finance_transactions SET category = NULL, category_source = NULL
WHERE category_source = 'rule' AND merchant ~ '^(DIRECT DEBIT|POUND STERLING)( |$)';

DELETE FROM finance_rules WHERE pattern IN ('DIRECT DEBIT', 'POUND STERLING');

-- Bank prefixes repeat after DIRECT DEBIT as they would before it, the same
-- loop as finance.noisePrefixes.
UPDATE finance_transactions
SET merchant = regexp_replace(merchant,
  '^((POS|DEBIT|CARD|PURCHASE|CHECKCARD|ACH|VISA|DDA|WITHDRAWAL|PREAUTHORIZED|RECURRING|TST|DIRECT DEBIT)( |$))+', '')
WHERE merchant ~ '^DIRECT DEBIT( |$)';

-- Only after a leading amount: the description's first word holds a digit.
UPDATE finance_transactions SET merchant = regexp_replace(merchant, '^POUND STERLING( |$)', '')
WHERE merchant ~ '^POUND STERLING( |$)' AND description ~ '^[^[:alnum:]&]*[[:alnum:]&]*[0-9]';

-- +goose Down
-- Nothing to undo: the older code matches rules against these shorter
-- merchants as it would any other, and the deleted rules were the bug.
SELECT 1;
