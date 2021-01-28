# Structural search performance testing

## Overview

The goal of this testing was to compare the performance of the old structural search code path to the new one. This runs a test for the product of each of the options defined in the test matrix.

To run: 
```bash
LOCAL_TOKEN=<token for dev env> CLOUD_TOKEN=<token for sourcegraph.com> go run .
```

A results directory will be created, and a subdirectory will be created for each test which will contain the profiling results if profiling is enabled for that endpoint (currently disabled for cloud). 

Results for each test run (latency, result count, etc.) are saved in a sqlite database in the directory. 

## Results

Most of the interesting results are from this query:
```sql
-- Compare old vs new
with new as (
  select * from results as r
  join test_cases as tc
  on r.test_case = tc.name
  where tc.new_codepath = 'new'
), old as (
  select * from results as r
  join test_cases as tc
  on r.test_case = tc.name
  where tc.new_codepath = 'old'
)
select 
  new.frontend_endpoint,
  new.result_set_size,
  new.query_trigger,
  new.count,
  new.repo,
  round(avg(new.took), 0) as new_took, 
  round(avg(old.took), 0) as old_took,
  count(new.error) as new_error_count,
  count(old.error) as old_error_count,
  count(distinct new.result_count) as new_unique_count,
  count(distinct old.result_count) as old_unique_count
from new
join old on 
  new.frontend_endpoint = old.frontend_endpoint AND
  new.result_set_size = old.result_set_size AND
  new.query_trigger = old.query_trigger AND
  new.count = old.count AND
  new.repo = old.repo
group by 
  new.frontend_endpoint,
  new.result_set_size,
  new.query_trigger,
  new.count,
  new.repo
order by new.name;
```

Output:
<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8"/><style>
table {border: medium solid #6495ed;border-collapse: collapse;width: 100%;} th{font-family: monospace;border: thin solid #6495ed;padding: 5px;background-color: #D0E3FA;}td{font-family: sans-serif;border: thin solid #6495ed;padding: 5px;text-align: center;}.odd{background:#e8edff;}img{padding:5px; border:solid; border-color: #dddddd #aaaaaa #aaaaaa #dddddd; border-width: 1px 2px 2px 1px; background-color:white;}</style>
</head>
<body>
<table><tr><th colspan="11">-- Compare old vs new </th></tr><tr><th>frontend_endpoint</th><th>result_set_size</th><th>query_trigger</th><th>count</th><th>repo</th><th>new_took</th><th>old_took</th><th>new_error_count</th><th>old_error_count</th><th>new_unique_count</th><th>old_unique_count</th></tr><tr class="odd"><td>cloud</td><td>lg</td><td>20x05s</td><td>10,000</td><td>chromium</td><td>8,954</td><td>33,644</td><td>340</td><td>120</td><td>4</td><td>1</td></tr>
<tr><td>cloud</td><td>md</td><td>20x05s</td><td>10,000</td><td>chromium</td><td>8,351</td><td>28,688</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>sm</td><td>20x05s</td><td>10,000</td><td>chromium</td><td>844</td><td>19,785</td><td>0</td><td>0</td><td>1</td><td>2</td></tr>
<tr><td>cloud</td><td>lg</td><td>20x05s</td><td>10,000</td><td>linux</td><td>2,940</td><td>30,188</td><td>380</td><td>0</td><td>2</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>md</td><td>20x05s</td><td>10,000</td><td>linux</td><td>14,490</td><td>22,350</td><td>0</td><td>0</td><td>5</td><td>1</td></tr>
<tr><td>cloud</td><td>sm</td><td>20x05s</td><td>10,000</td><td>linux</td><td>436</td><td>6,410</td><td>0</td><td>0</td><td>1</td><td>2</td></tr>
<tr class="odd"><td>cloud</td><td>lg</td><td>20x05s</td><td>10,000</td><td>sgtest</td><td>1,019</td><td>920</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>md</td><td>20x05s</td><td>10,000</td><td>sgtest</td><td>715</td><td>672</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>sm</td><td>20x05s</td><td>10,000</td><td>sgtest</td><td>433</td><td>472</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>lg</td><td>2x5s</td><td>10,000</td><td>chromium</td><td>9,134</td><td>0</td><td>0</td><td>4</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>md</td><td>2x5s</td><td>10,000</td><td>chromium</td><td>2,757</td><td>6,592</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>sm</td><td>2x5s</td><td>10,000</td><td>chromium</td><td>725</td><td>42,983</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>lg</td><td>2x5s</td><td>10,000</td><td>linux</td><td>9,069</td><td>34,705</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>md</td><td>2x5s</td><td>10,000</td><td>linux</td><td>2,605</td><td>15,914</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>sm</td><td>2x5s</td><td>10,000</td><td>linux</td><td>451</td><td>1,298</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>lg</td><td>2x5s</td><td>10,000</td><td>sgtest</td><td>927</td><td>1,048</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>md</td><td>2x5s</td><td>10,000</td><td>sgtest</td><td>628</td><td>743</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>sm</td><td>2x5s</td><td>10,000</td><td>sgtest</td><td>350</td><td>823</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>lg</td><td>20x05s</td><td>10,000</td><td>chromium</td><td>46,000</td><td>60,026</td><td>0</td><td>0</td><td>8</td><td>1</td></tr>
<tr><td>local</td><td>md</td><td>20x05s</td><td>10,000</td><td>chromium</td><td>1,605</td><td>59,156</td><td>0</td><td>0</td><td>1</td><td>3</td></tr>
<tr class="odd"><td>local</td><td>sm</td><td>20x05s</td><td>10,000</td><td>chromium</td><td>218</td><td>40,412</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>lg</td><td>20x05s</td><td>10,000</td><td>linux</td><td>59,262</td><td>60,023</td><td>0</td><td>0</td><td>3</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>md</td><td>20x05s</td><td>10,000</td><td>linux</td><td>13,453</td><td>42,719</td><td>0</td><td>0</td><td>1</td><td>3</td></tr>
<tr><td>local</td><td>sm</td><td>20x05s</td><td>10,000</td><td>linux</td><td>122</td><td>2,970</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>lg</td><td>20x05s</td><td>10,000</td><td>sgtest</td><td>585</td><td>488</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>md</td><td>20x05s</td><td>10,000</td><td>sgtest</td><td>349</td><td>307</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>sm</td><td>20x05s</td><td>10,000</td><td>sgtest</td><td>111</td><td>112</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>lg</td><td>2x5s</td><td>10,000</td><td>chromium</td><td>4,794</td><td>60,017</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>md</td><td>2x5s</td><td>10,000</td><td>chromium</td><td>1,221</td><td>21,245</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>sm</td><td>2x5s</td><td>10,000</td><td>chromium</td><td>229</td><td>20,121</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>lg</td><td>2x5s</td><td>10,000</td><td>linux</td><td>5,488</td><td>26,242</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>md</td><td>2x5s</td><td>10,000</td><td>linux</td><td>1,350</td><td>6,842</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>sm</td><td>2x5s</td><td>10,000</td><td>linux</td><td>129</td><td>3,454</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>lg</td><td>2x5s</td><td>10,000</td><td>sgtest</td><td>566</td><td>503</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>md</td><td>2x5s</td><td>10,000</td><td>sgtest</td><td>331</td><td>364</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>sm</td><td>2x5s</td><td>10,000</td><td>sgtest</td><td>127</td><td>157</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>lg</td><td>20x05s</td><td>10</td><td>chromium</td><td>8,852</td><td>19,373</td><td>0</td><td>0</td><td>3</td><td>4</td></tr>
<tr><td>cloud</td><td>md</td><td>20x05s</td><td>10</td><td>chromium</td><td>882</td><td>19,433</td><td>0</td><td>0</td><td>1</td><td>2</td></tr>
<tr class="odd"><td>cloud</td><td>sm</td><td>20x05s</td><td>10</td><td>chromium</td><td>816</td><td>19,354</td><td>0</td><td>0</td><td>1</td><td>2</td></tr>
<tr><td>cloud</td><td>lg</td><td>20x05s</td><td>10</td><td>linux</td><td>3,273</td><td>10,398</td><td>0</td><td>0</td><td>1</td><td>7</td></tr>
<tr class="odd"><td>cloud</td><td>md</td><td>20x05s</td><td>10</td><td>linux</td><td>715</td><td>2,608</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>sm</td><td>20x05s</td><td>10</td><td>linux</td><td>461</td><td>1,285</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>lg</td><td>20x05s</td><td>10</td><td>sgtest</td><td>955</td><td>851</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>md</td><td>20x05s</td><td>10</td><td>sgtest</td><td>668</td><td>607</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>sm</td><td>20x05s</td><td>10</td><td>sgtest</td><td>390</td><td>424</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>lg</td><td>2x5s</td><td>10</td><td>chromium</td><td>4,860</td><td>7,092</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>md</td><td>2x5s</td><td>10</td><td>chromium</td><td>841</td><td>4,540</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>sm</td><td>2x5s</td><td>10</td><td>chromium</td><td>1,009</td><td>9,059</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>lg</td><td>2x5s</td><td>10</td><td>linux</td><td>2,062</td><td>2,444</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>md</td><td>2x5s</td><td>10</td><td>linux</td><td>763</td><td>1,664</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>sm</td><td>2x5s</td><td>10</td><td>linux</td><td>543</td><td>3,471</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>lg</td><td>2x5s</td><td>10</td><td>sgtest</td><td>1,012</td><td>905</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>cloud</td><td>md</td><td>2x5s</td><td>10</td><td>sgtest</td><td>821</td><td>733</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>cloud</td><td>sm</td><td>2x5s</td><td>10</td><td>sgtest</td><td>515</td><td>595</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>lg</td><td>20x05s</td><td>10</td><td>chromium</td><td>8,626</td><td>50,194</td><td>0</td><td>0</td><td>5</td><td>4</td></tr>
<tr><td>local</td><td>md</td><td>20x05s</td><td>10</td><td>chromium</td><td>303</td><td>43,693</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>sm</td><td>20x05s</td><td>10</td><td>chromium</td><td>231</td><td>42,256</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>lg</td><td>20x05s</td><td>10</td><td>linux</td><td>1,219</td><td>13,848</td><td>0</td><td>0</td><td>1</td><td>6</td></tr>
<tr class="odd"><td>local</td><td>md</td><td>20x05s</td><td>10</td><td>linux</td><td>339</td><td>6,995</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>sm</td><td>20x05s</td><td>10</td><td>linux</td><td>127</td><td>2,910</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>lg</td><td>20x05s</td><td>10</td><td>sgtest</td><td>506</td><td>425</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>md</td><td>20x05s</td><td>10</td><td>sgtest</td><td>310</td><td>298</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>sm</td><td>20x05s</td><td>10</td><td>sgtest</td><td>111</td><td>110</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>lg</td><td>2x5s</td><td>10</td><td>chromium</td><td>2,713</td><td>22,285</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>md</td><td>2x5s</td><td>10</td><td>chromium</td><td>277</td><td>19,752</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>sm</td><td>2x5s</td><td>10</td><td>chromium</td><td>242</td><td>21,573</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>lg</td><td>2x5s</td><td>10</td><td>linux</td><td>1,102</td><td>4,483</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>md</td><td>2x5s</td><td>10</td><td>linux</td><td>365</td><td>3,634</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>sm</td><td>2x5s</td><td>10</td><td>linux</td><td>129</td><td>3,860</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>lg</td><td>2x5s</td><td>10</td><td>sgtest</td><td>523</td><td>460</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr class="odd"><td>local</td><td>md</td><td>2x5s</td><td>10</td><td>sgtest</td><td>335</td><td>400</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
<tr><td>local</td><td>sm</td><td>2x5s</td><td>10</td><td>sgtest</td><td>127</td><td>154</td><td>0</td><td>0</td><td>1</td><td>1</td></tr>
</table></body></html>


## Analysis

In almost all cases, the average query latency of the new code path is on par with or significantly lower than than the old code path. This could likely be further reduced by using the cached zip files when available.

It seems that the unique count is unstable with the new search when count is large. The only time unique count comes back greater than 1 when count is 10 is for the high-result-count chromium query, which is unsurprising. When count is large though, many files will be transferred, and it is timing out before all results can be processed.
