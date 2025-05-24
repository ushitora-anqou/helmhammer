local trimFunctions(x) =
  if std.isNumber(x) || std.isString(x) || std.isBoolean(x) || x == null then x
  else if std.isFunction(x) then null
  else if std.isArray(x) then std.map(trimFunctions, x)
  else if std.isObject(x) then std.mapWithKey(function(k, v) trimFunctions(v), x);

local objectRemoveKey(obj, key) = {
  // cf. https://github.com/google/go-jsonnet/issues/808
  [k]: obj[k]
  for k in std.objectFields(obj)
  if k != key
};

local allocate(heap, v) =
  local
    pointer = std.toString(std.length(heap)),
    heap1 = heap { [pointer]: v };
  [heap1, { p: pointer }];

local isAddr(v) =
  std.isObject(v) && std.length(v) == 1 && std.objectHas(v, 'p');

local deref(heap, addr) =
  if isAddr(addr) then heap[addr.p] else error ('deref: not addr: %s' % [addr]);

local assign(heap, addr, v) =
  if isAddr(addr) then
    heap { [addr.p]: v }
  else
    error ('assign: invalid addr: %s' % [trimFunctions(addr)]);

local arrayReplace(ary, index, newItem) =
  std.mapWithIndex(
    function(i, item) if i == index then newItem else item,
    ary,
  );

local fromConst(heap, src) =
  if src == null || std.isNumber(src) || std.isString(src) || std.isBoolean(src) then
    [heap, src]
  else
    local aux(heap, queue0, out) =
      local
        first = queue0[0],
        queue = queue0[1],
        src = first[0],
        k = first[1];
      if std.length(queue0) == 0 then [heap, out]
      else if src == null || std.isNumber(src) || std.isString(src) || std.isBoolean(src) then
        local res = k(heap, src, out), heap1 = res[0], out1 = res[1];
        aux(heap1, queue, out1) tailstrict
      else if std.isFunction(src) then
        local res = allocate(heap, src), heap1 = res[0], v = res[1];
        local res = k(heap1, v, out), heap2 = res[0], out1 = res[1];
        aux(heap2, queue, out1) tailstrict
      else if std.isArray(src) then
        local res = allocate(heap, src), heap1 = res[0], aryp = res[1];
        local res = k(heap1, aryp, out), heap2 = res[0], out1 = res[1];
        local queue1 =
          std.foldl(
            function(queue, x) [x, queue],
            std.mapWithIndex(
              function(index, item)
                [
                  item,
                  function(heap, itemv, out)
                    [
                      assign(
                        heap,
                        aryp,
                        arrayReplace(deref(heap, aryp), index, itemv),
                      ),
                      out,
                    ],
                ],
              src,
            ),
            queue,
          );
        aux(heap2, queue1, out1) tailstrict
      else if std.isObject(src) then
        local res = allocate(heap, src), heap1 = res[0], objp = res[1];
        local res = k(heap1, objp, out), heap2 = res[0], out1 = res[1];
        local queue1 =
          std.foldl(
            function(queue, x) [x, queue],
            std.map(
              function(key)
                [
                  src[key],
                  function(heap, value, out)
                    if src[key] == value then [heap, out]
                    else [
                      assign(
                        heap,
                        objp,
                        deref(heap, objp) + { [key]: value },
                      ),
                      out,
                    ],
                ],
              std.objectFields(src),
            ),
            queue,
          );
        aux(heap2, queue1, out1) tailstrict
      else
        error 'fromConst: unknown type';
    aux(
      heap,
      [
        [
          src,
          function(heap, itemv, _out) [
            heap,
            itemv,  // set out
          ],
        ],
        [],
      ],
      null,
    ) tailstrict;

local toConst(heap, src) =
  if src == null || std.isNumber(src) || std.isString(src) || std.isBoolean(src) then
    src
  else
    local aux(heap, src) =
      if isAddr(src) then
        local v = deref(heap, src);
        if std.isFunction(v) then
          v
        else if std.isArray(v) then
          std.map(function(item) aux(heap, item), v)
        else if std.isObject(v) then
          std.mapWithKey(function(_, src) aux(heap, src), v)
        else
          error 'toConst: invalid addr'
      else if src == null || std.isNumber(src) || std.isString(src) || std.isBoolean(src) then
        src
      else
        error 'toConst: invalid value. maybe already const?';
    aux(heap, src) tailstrict;

local field(heap, receiver0, fieldName, args) =
  local receiver =
    if isAddr(receiver0)
    then deref(heap, receiver0)
    else receiver0;
  assert !isAddr(receiver);
  //assert (
  //  if isAddr(receiver)
  //  then std.trace("field: %s %s" % [receiver0, trimFunctions(heap)], false)
  //  else true);
  if std.isObject(receiver) && std.objectHas(receiver, fieldName) then
    if isAddr(receiver[fieldName]) &&
       std.isFunction(deref(heap, receiver[fieldName]))
    then
      // FIXME: allow to return allocated pointer
      deref(heap, receiver[fieldName])(heap, args)
    else if std.length(args) != 0 then
      error ('field: invalid arguments: %s' % [fieldName])
    else
      [heap, receiver[fieldName]]  // return non-dereferenced value
  else
    if std.length(args) != 0 then
      error ('field: invalid arguments: %s' % [fieldName])
    else
      [heap, null];
//std.trace('%s %s' % [trimFunctions(receiver), fieldName], null),

local isTrueOnHeap(heap, v) =
  if isAddr(v) then
    local w = deref(heap, v);
    if std.isArray(w) || std.isObject(w) then std.length(w) > 0
    else if std.isFunction(v) then true
    else error 'isTrueOnHeap: invalid type of address'
  else if v == null then false
  else if std.isString(v) then std.length(v) > 0
  else if std.isBoolean(v) then v
  else if std.isNumber(v) then v != 0
  else error 'isTrueOnHeap: invalid type of value';

local range(vs0, heap0, values0, fthen, felse) =
  if values0 == null then felse()
  else if std.isNumber(values0) then
    local
      res = allocate(heap0, std.makeArray(values0, function(x) x)),
      heap = res[0],
      aryp = res[1];
    range(vs0, heap, aryp, fthen, felse)
  else if isAddr(values0) then
    local values = deref(heap0, values0);
    if std.isArray(values) then
      if std.length(values) == 0 then felse()
      else
        local res = std.foldl(
          function(acc, value)
            local res = fthen(acc.vs, acc.h, acc.i, value);
            {
              i: acc.i + 1,
              v: acc.v + res[0],
              vs: res[1],
              h: res[2],
            },
          values,
          {
            i: 0,
            v: '',
            vs: vs0,
            h: heap0,
          },
        );
        [res.v, res.vs, res.h]
    else if std.isObject(values) then
      if std.length(values) == 0 then felse()
      else
        local res = std.foldl(
          function(acc, kv)
            local res = fthen(acc.vs, acc.h, kv.key, kv.value);
            {
              i: acc.i + 1,
              v: acc.v + res[0],
              vs: res[1],
              h: res[2],
            },
          std.objectKeysValues(values),
          {
            i: 0,
            v: '',
            vs: vs0,
            h: heap0,
          },
        );
        [res.v, res.vs, res.h]
    else error ('range: not implemented: %s' % [values0])
  else error ('range: not implemented: %s' % [values0]);

local printf(args) =
  std.format(args[0], args[1:]);

local contains(args) =
  std.findSubstr(args[0], args[1]) != [];

local trimSuffix(args) =
  if std.endsWith(args[1], args[0]) then
    std.substr(args[1], 0, std.length(args[1]) - std.length(args[0]))
  else
    args[1];

local trunc(args) =
  assert std.length(args) == 2;
  assert std.isNumber(args[0]);
  assert std.isString(args[1]);
  if args[0] >= 0 then
    std.substr(args[1], 0, args[0])
  else
    std.substr(args[1], std.length(args[1]) + args[0], -args[0]);

local indent(args) =
  std.join(
    '\n',
    std.map(
      function(x) std.repeat(' ', args[0]) + x,
      std.split(args[1], '\n'),
    ),
  );

local nindent(args) =
  '\n' + indent(args);

local replace(args) =
  std.strReplace(args[2], args[0], args[1]);

local quote(args) =
  std.join(
    ' ',
    std.map(
      function(x) '"%s"' % [std.strReplace(x, '"', '\\"')],
      std.filterMap(function(x) x != null, std.toString, args),
    ),
  );

local squote(args) =
  std.join(
    ' ',
    std.map(
      function(x) "'%s'" % [std.strReplace(x, "'", "\\'")],
      std.filterMap(function(x) x != null, std.toString, args),
    ),
  );

local eq(args) =
  assert std.length(args) == 2;
  args[0] == args[1];

local ne(args) =
  assert std.length(args) == 2;
  args[0] != args[1];

local print(args) =
  // Equivalent to fmt.Sprint of Go.
  //
  // > Sprint formats using the default formats for its operands and
  // > returns the resulting string. Spaces are added between operands
  // > when neither is a string.
  local aux(i, out) =
    if i >= std.length(args) then out
    else if std.isString(args[i]) then aux(i + 1, out + args[i])
    else if i >= 1 && !std.isString(args[i - 1]) then
      aux(i + 1, out + ' ' + std.toString(args[i]))
    else
      aux(i + 1, out + std.toString(args[i]));
  aux(0, '');

local concat(args) =
  std.join([], args);

local lower(args) =
  assert std.length(args) == 1;
  std.asciiLower(args[0]);

local required(args) =
  assert std.length(args) == 2;
  // FIXME
  if args[1] == null then error args[0];

local sha256sum(args) =
  assert std.length(args) == 1;
  std.sha256(args[0]);

// cf. https://github.com/google/jsonnet/blob/42153e4c993c2b8196f98c5ab6f1150f398e3d0d/stdlib/std.jsonnet#L1000
local escapeStringJsonSQuote(str_) =
  local str = std.toString(str_);
  local trans(ch) =
    if ch == "'" then
      "\\'"
    else if ch == '\\' then
      '\\\\'
    else if ch == '\b' then
      '\\b'
    else if ch == '\f' then
      '\\f'
    else if ch == '\n' then
      '\\n'
    else if ch == '\r' then
      '\\r'
    else if ch == '\t' then
      '\\t'
    else
      local cp = std.codepoint(ch);
      if cp < 32 || (cp >= 127 && cp <= 159) then
        '\\u%04x' % [cp]
      else
        ch;
  "'%s'" % std.join('', [trans(ch) for ch in std.stringChars(str)]);

local toYaml(args) =
  std.manifestYamlDoc(args[0], quote_keys=false);

local dir(args) =
  assert std.length(args) == 1;
  std.join('/', std.split(args[0], '/')[0:-1]);

local toInt(v) =
  if std.isNumber(v) then v
  else if std.isString(v) then std.parseInt(v)
  else error 'toInt: not number nor string';

local min(args) =
  assert std.length(args) >= 1;
  std.minArray(std.map(toInt, args));

local hasKey(args) =
  assert std.length(args) == 2;
  assert std.isString(args[1]);
  if args[0] == null then false
  else
    assert std.isObject(args[0]);
    std.objectHas(args[0], args[1]);

local b64enc(args) =
  assert std.length(args) == 1;
  assert std.isString(args[0]);
  std.base64(args[0]);

local gt(args) =
  assert std.length(args) == 2;
  args[0] > args[1];

local int(args) =
  assert std.length(args) == 1;
  toInt(args[0]);

local toString(args) =
  assert std.length(args) == 1;
  std.toString(args[0]);

local has(args) =
  assert std.length(args) == 2;
  local needle = args[0], haystack = args[1];
  assert std.isArray(haystack);
  std.member(haystack, needle);

local fail(args) =
  assert std.length(args) == 1;
  assert std.isString(args[0]);
  error ('fail: %s' % [args[0]]);

local trimAll(args) =
  assert std.length(args) == 2;
  assert std.isString(args[0]);
  assert std.isString(args[1]);
  local
    trimLeft(s, cutset) =
      local loop(i) =
        if i >= std.length(s) || !std.member(cutset, s[i]) then i
        else loop(i + 1);
      s[loop(0):],
    trimRight(s, cutset) =
      local loop(i) =
        if i < 0 || !std.member(cutset, s[i]) then i
        else loop(i - 1);
      s[0:loop(std.length(s) - 1) + 1],
    cutset = args[0],
    s = args[1],
    s1 = trimLeft(s, cutset),
    s2 = trimRight(s1, cutset);
  s2;

local parseYaml(src) =
  // avoid a go-jsonnet's known issue:
  // https://github.com/google/go-jsonnet/issues/714
  if src == '' then null
  else std.parseYaml(src);

local fromYaml(args) =
  assert std.length(args) == 1;
  assert std.isString(args[0]);
  parseYaml(args[0]);

local int64(args) =
  assert std.length(args) == 1;
  local v = args[0];
  if v == null then 0
  else if std.isNumber(v) then v
  else if std.isString(v) then std.parseInt(v)
  else if std.isBoolean(v) then if v then 1 else 0
  else error 'int64: invalid type';

local trim(args) =
  assert std.length(args) == 1;
  assert std.isString(args[0]);
  std.trim(args[0]);

local omit(args) =
  assert std.length(args) >= 1;
  assert std.isObject(args[0]);
  std.foldl(objectRemoveKey, args[1:], args[0]);

local regexReplaceAll(args) =
  assert std.length(args) == 3;
  assert std.isString(args[0]);
  assert std.isString(args[1]);
  assert std.isString(args[2]);
  if args[0] == '[^-A-Za-z0-9_.]' && args[1] == 'v2.14.11' then
    'v2.14.11'
  else if args[0] == '(.*)(@sha.*)' && std.findSubstr('@sha', args[1]) == [] && args[2] == '${1}' then
    args[1]
  else
    error ('regexReplaceAll: not implemented: %s' % [args]);

local mustRegexReplaceAllLiteral(args) =
  assert std.length(args) == 3;
  assert std.isString(args[0]);
  assert std.isString(args[1]);
  assert std.isString(args[2]);
  if args[1] == '' then ''
  else error ('mustRegexReplaceAllLiteral: not implemented: %s' % [args]);

local regexReplaceAllLiteral(args) =
  assert std.length(args) == 3;
  assert std.isString(args[0]);
  assert std.isString(args[1]);
  assert std.isString(args[2]);
  if args[0] == '[^a-zA-Z0-9._-]' && args[1] == '3.4.2' then
    '3.4.2'
  else error ('regexReplaceAllLiteral: not implemented: %s' % [trimFunctions(args)]);

local ternary(args) =
  assert std.length(args) == 3;
  assert std.isBoolean(args[2]);
  if args[2] then args[0] else args[1];

local semverCompare(args) =
  // FIXME
  if (args[0] == '>=1.13-0' || args[0] == '>= 1.23-0' || args[0] == '>=1.21-0') && args[1] == 'v1.32.0' then
    true
  else if args[0] == '<3.14.0' && args[1] == 'v3.17' then
    false
  else if args[0] == '>=1.4.0-0' && args[1] == '1.9.1' then
    true
  else
    error ('semverCompare: not implemented: %s' % [args]);

local add(args) =
  assert std.length(args) >= 2;
  std.foldl(function(acc, arg) acc + toInt(arg), args, 0);

local mul(args) =
  assert std.length(args) >= 2;
  std.foldl(function(acc, arg) acc * toInt(arg), args, 1);

local div(args) =
  assert std.length(args) == 2;
  toInt(args[0]) / toInt(args[1]);

local add1(args) = error ('add1: not implemented: %s' % [trimFunctions(args)]);
local ceil(args) = error ('ceil: not implemented: %s' % [trimFunctions(args)]);
local clean(args) = error ('clean: not implemented: %s' % [trimFunctions(args)]);
local dateInZone(args) = error 'dateInZone: not implemented';
local divf(args) = error ('divf: not implemented: %s' % [trimFunctions(args)]);
local first(args0) = error 'first: not implemented';
local fromJson(args) = error ('fromJson: not implemented: %s' % [trimFunctions(args)]);
local fromYamlArray(args) = error ('fromYamlArray: not implemented: %s' % [trimFunctions(args)]);
local ge(args0) = error 'ge: not implemented';
local genCA(args) = error ('genCA: not implemented: %s' % [trimFunctions(args)]);
local genSignedCert(args) = error ('genSignedCert: not implemented: %s' % [trimFunctions(args)]);
local hasSuffix(args) = error ('hasSuffix: not implemented: %s' % [trimFunctions(args)]);
local lt(args) = error ('lt: not implemented: %s' % [trimFunctions(args)]);
local mulf(args) = error ('mulf: not implemented: %s' % [trimFunctions(args)]);
local mustUniq(args) = error ('mustUniq: not implemented: %s' % [trimFunctions(args)]);
local now(args) = error 'now: not implemented';
local randAlphaNum(args0) = error 'randAlphaNum: not implemented';
local regexFind(args0) = error 'regexMatch: not implemented';
local regexMatch(args0) = error 'regexMatch: not implemented';
local reverse(args0) = error 'reverse: not implemented';
local sub(args0) = error 'sub: not implemented';
local toJson(args) = error ('toJson: not implemented: %s' % [trimFunctions(args)]);
local typeIs(args) = error 'typeIs: not implemented';
local typeOf(args0) = error 'typeOf: not implemented';
local urlParse(args0) = error 'urlParse: not implemented';

local _empty(heap, v) =
  if v == null then
    true
  else if isAddr(v) then
    local w = deref(heap, v);
    assert std.isArray(w) || std.isObject(w);
    std.length(w) == 0
  else if std.isString(v) then
    std.length(v) == 0
  else if std.isBoolean(v) then
    !v
  else if std.isNumber(v) then
    v == 0;

local until(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  local count = args[0];
  assert std.isNumber(count);
  if count < 0 then error "until: not implemented"
  else
    local v = std.range(0, count - 1);
    local res = allocate(heap, v), heap1 = res[0], p = res[1];
    [p, vs, heap1];

local keys(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  local v = std.flattenArrays(std.map(function(dict) std.objectFields(dict), args));
  local res = allocate(heap, v), heap1 = res[0], p = res[1];
  [p, vs, heap1];

local _strval(heap, x) =
  if x == null then 'null'
  else if std.isString(x) then x
  else if std.isNumber(x) || std.isBoolean(x) then std.toString(x)
  else error '_strval: not implemented';

local _join(heap, ary) =
  std.join(
    '',
    std.map(
      function(x) _strval(heap, x),
      ary,
    ),
  );

local _strslice(heap, v) =
  if isAddr(v) then
    local dv = deref(heap, v);
    if std.isArray(dv) then std.map(function(x) _strval(heap, x), dv)
    else error '_strslice: invalid argument'
  else if std.isString(v) then [v]
  else error '_strslice: invalid argument';

local sortAlpha(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  local a = _strslice(heap, args[0]);
  local res = allocate(heap, std.sort(a)), heap1 = res[0], p = res[1];
  [p, vs, heap1];

local split(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 2;
  local sep = args[0], orig = args[1];
  assert std.isString(sep);
  assert std.isString(orig);
  local parts = std.split(orig, sep);
  local v = { ['_' + i]: parts[i] for i in std.range(0, std.length(parts) - 1) };
  local res = allocate(heap, v), heap1 = res[0], objP = res[1];
  [objP, vs, heap];

local splitList(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 2;
  local sep = args[0], orig = args[1];
  assert std.isString(sep);
  assert std.isString(orig);
  local parts = std.split(orig, sep);
  local res = allocate(heap, parts), heap1 = res[0], aryP = res[1];
  [aryP, vs, heap1];

local upper(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  local str = args[0];
  [std.asciiUpper(str), vs, heap];

local lookup(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  local res = fromConst(heap, {}), heap1 = res[0], objP = res[1];
  [objP, vs, heap1];

local compact(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  local list = deref(heap, args[0]);
  assert std.isArray(list);
  local filtered = std.filter(function(x) !_empty(heap, x), list);
  local res = allocate(heap, filtered), heap1 = res[0], filteredP = res[1];
  [filteredP, vs, heap1];

local untitle(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  local str = args[0];
  assert std.isString(str);
  local v = std.join(
    '',
    std.map(
      function(word)
        if word == '' then ''
        else std.asciiLower(word[0]) + word[1:],
      std.split(str, ' '),
    ),
  );
  [v, vs, heap];

local kebabcase(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  local str = args[0];
  assert std.isString(str);
  local lAlphaNum = std.set(std.stringChars('abcdefghijklmnopqrstuvwxyz0123456789'));
  local uAlpha = std.set(std.stringChars('ABCDEFGHIJKLMNOPQRSTUVWXYZ'));
  local aux(i, out) =
    if i >= std.length(str) then out
    else if str[i] == '-' then
      aux(i + 1, out + '-') tailstrict
    else if std.setMember(str[i], lAlphaNum) then
      aux(i + 1, out + str[i]) tailstrict
    else if std.setMember(str[i], uAlpha) then
      aux(i + 1, out + '-' + std.asciiLower(str[i])) tailstrict
    else
      error ('kebabcase: not implemented: %s' % [str]);
  [aux(0, ''), vs, heap];

local camelcase(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  local str = args[0];
  assert std.isString(str);
  local strSet = std.set(std.stringChars(str));
  local lAlphaSet = std.set(std.stringChars('abcdefghijklmnopqrstuvwxyz0123456789'));
  local lAlphaMinusSet = std.set(std.stringChars('abcdefghijklmnopqrstuvwxyz0123456789-'));
  if std.setDiff(strSet, lAlphaSet) == [] then
    [std.asciiUpper(str[0]) + str[1:], vs, heap]
  else if std.setDiff(strSet, lAlphaMinusSet) == [] then
    [std.join('', std.map(function(x) std.asciiUpper(x), std.split(str, '-'))), vs, heap]
  else error ('camelcase: not implemented: %s' % [str]);

local join(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 2;
  local sep = args[0];
  assert std.isString(sep);
  [std.join(sep, _strslice(heap, args[1])), vs, heap];

local append(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 2;
  local list = deref(heap, args[0]), v = args[1];
  assert std.isArray(list);
  local newList = list + [v];
  local res = allocate(heap, newList), heap1 = res[0], newListP = res[1];
  [newListP, vs, heap1];

local dig(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) >= 3;
  local keys = args[0:std.length(args) - 2], default = args[std.length(args) - 2], dict = args[std.length(args) - 1];
  local aux(i, cur) =
    if i == std.length(keys) then cur
    else if keys[i] in cur then aux(i + 1, cur[keys[i]]) tailstrict
    else default;
  local v = aux(0, dict);
  [v, vs, heap];

local hasPrefix(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  local v = std.startsWith(args[1], args[0]);
  [v, vs, heap];

local _deepEqual(x, y) =
  if isAddr(x) || isAddr(y) then error '_deepEqual: not implemented'
  else x == y;

local without(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) >= 1;
  local list = deref(heap, args[0]), omit = args[1:];
  local filtered =
    std.filter(
      function(x)
        !std.any(std.map(function(y) _deepEqual(x, y), omit)),
      list,
    );
  local res = allocate(heap, filtered), heap1 = res[0], newListP = res[1];
  [newListP, vs, heap1];

local _kindOf(heap, v) =
  if v == null then 'invalid'
  else if std.isString(v) then 'string'
  else if std.isNumber(v) then 'float64'
  else if std.isBoolean(v) then 'bool'
  else if std.isObject(deref(heap, v)) then 'map'
  else if std.isArray(deref(heap, v)) then 'array'
  else if std.isFunction(deref(heap, v)) then 'func'
  else 'invalid';

local kindIs(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 2;
  assert std.isString(args[0]);
  local v = args[0] == _kindOf(heap, args[1]);
  [v, vs, heap];

local kindOf(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  [_kindOf(heap, args[0]), vs, heap];

local b64dec(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  assert std.isString(args[0]);
  local v = std.base64DecodeBytes(args[0]);
  [v, vs, heap];

local toRawJson(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  local v = std.manifestJson(toConst(heap, args[0]));
  [v, vs, heap];

local unset(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 2;
  local objp = args[0], key = args[1];
  assert std.isString(key);
  assert isAddr(objp);
  local objv = deref(heap, objp);
  assert std.isObject(objv);
  local newobjv = objectRemoveKey(objv, key);
  local newheap = assign(heap, objp, newobjv);
  [objp, vs, newheap];

local base_(path) =
  assert std.isString(path);
  if path == '' then '.'
  else if path == '/' then '/'
  else
    local parts = std.findSubstr('/', path);
    if std.length(parts) == 0 then path
    else path[parts[std.length(parts) - 1] + 1:];

local base(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  [base_(args[0]), vs, heap];

local ext_(path) =
  assert std.isString(path);
  local dots = std.findSubstr('.', path);
  if std.length(dots) == 0 then ''
  else path[dots[std.length(dots) - 1]:];

local ext(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  [ext_(args[0]), vs, heap];

local regexSplit(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 3;
  local regex = args[0], s = args[1], n = args[2];
  assert std.isString(regex);
  assert std.isString(s);
  assert std.isNumber(n);
  // FIXME: implement
  if regex == ':' && n == -1 then
    local res = fromConst(heap, std.split(s, ':')), newheap = res[0], v = res[1];
    [v, vs, newheap]
  else if regex == '[-_.]' && n == -1 then
    local ary1 = std.split(s, '-');
    local ary2 = std.flattenArrays(std.map(function(s) std.split(s, '_'), ary1));
    local ary3 = std.flattenArrays(std.map(function(s) std.split(s, '.'), ary2));
    local res = fromConst(heap, ary3), newHeap = res[0], v = res[1];
    [v, vs, newHeap]
  else
    error 'regexSplit: not implemented: %s' % [args];

local last(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  assert isAddr(args[0]);
  local list = deref(heap, args[0]);
  assert std.isArray(list);
  if std.length(list) == 0 then [null, vs, heap]
  else [list[std.length(list) - 1], vs, heap];

local uniq(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  local listp = args[0];
  assert isAddr(listp);
  local list = deref(heap, listp);
  assert std.isArray(list);
  local newlist = std.uniq(list);
  local newheap = assign(heap, listp, newlist);
  [listp, vs, newheap];

local get(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 2;
  assert isAddr(args[0]);
  local d = deref(heap, args[0]);
  assert std.isObject(d);
  assert std.isString(args[1]);
  local key = args[1];
  local retv = if std.objectHas(d, key) then d[key] else '';
  [retv, vs, heap];

local coalesce(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  local aux(i) =
    if i >= std.length(args) then null
    else if _empty(heap, args[i]) then aux(i + 1)
    else args[i];
  [aux(0), vs, heap];

local len(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  if isAddr(args[0]) then
    [std.length(deref(heap, args[0])), vs, heap]
  else if std.isString(args[0]) then
    [std.length(args[0]), vs, heap]
  else
    error 'len: invalid type';

local not(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  [!isTrueOnHeap(heap, args[0]), vs, heap];

local or(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) >= 1;
  local loop(i) =
    if i == std.length(args) - 1 || isTrueOnHeap(heap, args[i]) then args[i]
    else loop(i + 1);
  [loop(0), vs, heap];

local and(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) >= 1;
  local loop(i) =
    if i == std.length(args) - 1 || !isTrueOnHeap(heap, args[i]) then args[i]
    else loop(i + 1);
  [loop(0), vs, heap];

local empty(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  [_empty(heap, args[0]), vs, heap];

local default(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) >= 1;
  if std.length(args) == 1 || _empty(heap, args[1]) then
    [args[0], vs, heap]
  else
    [args[1], vs, heap];

local list(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  local res = allocate(heap, args);
  [res[1], vs, res[0]];

local tuple(args0) =
  list(args0);

local index(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) >= 2;
  local v = std.foldl(
    function(addr, arg)
      if addr == null then null
      else
        local v = deref(heap, addr);
        if std.isObject(v) then
          if !std.isString(arg) then error 'index: key is not a string'
          else if std.objectHas(v, arg) then v[arg]
          else null
        else if std.isArray(v) then
          if !std.isNumber(arg) then error 'index: key is not an integer'
          else if arg < std.length(v) then v[arg]
          else null
        else null,
    args[1:],
    args[0],
  );
  [v, vs, heap];

local include(args0) =
  local templates = args0['$'], args = args0.args, vs = args0.vs, heap = args0.h;
  local res = templates[args[0]](heap, args[1]);
  [res[0], vs, res[2]];

local deepCopy(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 1;
  local
    res = fromConst(heap, toConst(heap, args[0])),
    newheap = res[0],
    v = res[1];
  [v, vs, newheap];

local dict(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  local loop(i, out) =
    if i >= std.length(args) then out
    else
      assert !isAddr(args[i]);
      local key = std.toString(args[i]);
      if i + 1 >= std.length(args) then
        loop(i + 2, out { [key]: '' })
      else
        loop(i + 2, out { [key]: args[i + 1] });
  local m = loop(0, {});
  local res = allocate(heap, m), heap1 = res[0], v = res[1];
  [v, vs, heap1];

local mergeOverwrite(args0) =
  // FIXME: implement mergo
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) >= 1;
  assert std.all(
    std.map(
      function(arg) isAddr(arg) && std.isObject(deref(heap, arg)),
      args,
    ),
  );
  local constArgs = std.map(function(arg) toConst(heap, arg), args);
  local merged = std.foldl(std.mergePatch, constArgs[1:], constArgs[0]);
  local res = fromConst(heap, merged), heap1 = res[0], p = res[1];
  local newheap = assign(heap1, args[0], deref(heap1, p));
  [p, vs, newheap];

local merge(args0) =
  // FIXME: implement mergo
  mergeOverwrite(args0);

local mustMerge(args0) =
  // FIXME: implement mergo
  merge(args0);

local _set(heap, objp, key, newValue) =
  assert std.isString(key);
  assert isAddr(objp);
  local objv = deref(heap, objp);
  assert std.isObject(objv);
  local newobjv = objv { [key]: newValue };
  assign(heap, objp, newobjv);

local set(args0) =
  local args = args0.args, vs = args0.vs, heap = args0.h;
  assert std.length(args) == 3;
  local objp = args[0], key = args[1], newValue = args[2];
  local newheap = _set(heap, objp, key, newValue);
  [objp, vs, newheap];

local callBuiltin(h, f, args) =
  fromConst(
    h,
    f(std.map(function(arg) toConst(h, arg), args)),
  );

local strIndex(pat, str, start) =
  // FIXME: slow
  local parts = std.split(str, pat);
  local loop(i, pos) =
    if i >= std.length(parts) then -1
    else if i == 0 then
      loop(i + 1, pos + std.length(parts[i])) tailstrict
    else if pos < start then
      loop(i + 1, pos + std.length(pat) + std.length(parts[i])) tailstrict
    else pos;
  loop(0, 0) tailstrict;

local tpl_(templates) =
  {
    local isSpace(c) =
      c == ' ' || c == '\n' || c == '\r' || c == '\t',

    local findNonSpace(str, i, step) =
      local c = str[i];
      if i < 0 || i >= std.length(str) then
        i
      else if isSpace(c) then
        findNonSpace(str, i + step, step)
      else
        i,

    local lexText(str, i0, out, skipLeadingSpaces) =
      assert i0 < std.length(str) : 'lexText: unexpected eof';
      local i =
        if skipLeadingSpaces then findNonSpace(str, i0, 1)
        else i0;
      if i >= std.length(str) then out
      else
        /*
                  0 1 2 3 4 5
                  a   { { - a
          i     = 0
          j     =     2
          j - 1 =   1
          j + 2 =         4
          j + 3 =           5
          k     = 0
          k + 1 =   1
        */
        local j = strIndex('{{', str, i);
        if j == -1 then out + [{ t: 'text', v: str[i:] }]
        else
          assert j + 2 < std.length(str) : 'lexText: unexpected {{';
          if str[j + 2] == '-' then
            local k = findNonSpace(str, j - 1, -1);
            lexInsideAction(
              str,
              j + 3,
              (if i >= k + 1 then out else out + [{ t: 'text', v: str[i:k + 1] }]) + [{ t: '{{' }]
            ) tailstrict
          else
            lexInsideAction(
              str,
              j + 2,
              (if i >= j then out else out + [{ t: 'text', v: str[i:j] }]) + [{ t: '{{' }]
            ) tailstrict,

    local isAlphanumeric(ch) =
      local c = std.codepoint(ch);
      ch == '_' ||
      std.codepoint('a') <= c && c <= std.codepoint('z') ||
      std.codepoint('A') <= c && c <= std.codepoint('Z') ||
      std.codepoint('0') <= c && c <= std.codepoint('9'),

    local isNumeric(ch) =
      local c = std.codepoint(ch);
      std.codepoint('0') <= c && c <= std.codepoint('9'),

    // lexFieldOrVariable scans a field or variable: [.$]Alphanumeric.
    // The . or $ has been scanned.
    local lexFieldOrVariable(str, i) =
      local
        loop(i) =
          if i >= std.length(str) then error 'lexFieldOrVariable: unexpected eof'
          else if isAlphanumeric(str[i]) then loop(i + 1) tailstrict
          else i,
        j = loop(i);
      [j, str[i:j]],

    local lexIdentifier(str, i) =
      local
        loop(i) =
          if i >= std.length(str) then error 'lexIdentifier: unexpected eof'
          else if isAlphanumeric(str[i]) then loop(i + 1) tailstrict
          else i,
        j = loop(i);
      [j, str[i:j]],

    local lexNumber(str, i) =
      local
        loop(i) =
          if i >= std.length(str) then error 'lexNumber: unexpected eof'
          else if isNumeric(str[i]) then loop(i + 1) tailstrict
          else i,
        j = loop(i);
      [j, std.parseInt(str[i:j])],

    local lexString(str, i, quote) =  // FIXME: escape
      local
        loop(j) =
          if j >= std.length(str) then error 'lexString: unexpected eof'
          else if str[j] == '\\' && str[j + 1] == quote then [j + 2, j]
          else if str[j] == quote then [j + 1, j]
          else loop(j + 1) tailstrict,
        res = loop(i + 1),
        j1 = res[0],
        j2 = res[1];
      [j1, str[i + 1:j2]],

    local lexInsideAction(str, i, out) =
      if i + 2 < std.length(str) && str[i] == '-' && str[i + 1] == '}' && str[i + 2] == '}' then
        lex(str, i + 3, out + [{ t: '}}' }], skipLeadingSpaces=true)
      else if i + 1 < std.length(str) && str[i] == '}' && str[i + 1] == '}' then
        lex(str, i + 2, out + [{ t: '}}' }])
      else
        local c = str[i];
        if c == '.' then
          local res = lexFieldOrVariable(str, i + 1), j = res[0], v = res[1];
          lexInsideAction(str, j, out + [{ t: 'field', v: v }]) tailstrict
        else if c == '$' then
          local res = lexFieldOrVariable(str, i + 1), j = res[0], v = res[1];
          lexInsideAction(str, j, out + [{ t: 'var', v: v }]) tailstrict
        else if c == '|' || c == '(' || c == ')' then
          lexInsideAction(str, i + 1, out + [{ t: c }]) tailstrict
        else if isSpace(c) then
          lexInsideAction(str, findNonSpace(str, i + 1, 1), out + [{ t: ' ' }]) tailstrict
        else if c == '=' then
          lexInsideAction(str, i + 1, out + [{ t: '=' }]) tailstrict
        else if c == ':' && str[i + 1] == '=' then
          lexInsideAction(str, i + 2, out + [{ t: ':=' }]) tailstrict
        else if c == '\\' && (str[i + 1] == '"' || str[i + 1] == '`') then
          local res = lexString(str, i + 1, str[i + 1]), j = res[0], v = res[1];
          lexInsideAction(str, j, out + [{ t: 'string', v: v }]) tailstrict
        else if c == '"' || c == '`' then
          local res = lexString(str, i, c), j = res[0], v = res[1];
          lexInsideAction(str, j, out + [{ t: 'string', v: v }]) tailstrict
        else if isNumeric(c) then
          local res = lexNumber(str, i), j = res[0], v = res[1];
          lexInsideAction(str, j, out + [{ t: 'number', v: v }]) tailstrict
        else if isAlphanumeric(c) then
          local res = lexIdentifier(str, i), j = res[0], v = res[1];
          local token =
            if v == 'with' then { t: 'with' }
            else if v == 'if' then { t: 'if' }
            else if v == 'else' then { t: 'else' }
            else if v == 'end' then { t: 'end' }
            else if v == 'true' then { t: 'true' }
            else if v == 'false' then { t: 'false' }
            else if v == 'range' then { t: 'range' }
            else { t: 'id', v: v };
          lexInsideAction(str, j, out + [token]) tailstrict
        else error ('lexInsideAction: unexpected char: %s' % [c]),

    local lex(str, i, out, skipLeadingSpaces=false) =
      if i >= std.length(str) then
        out
      else
        lexText(str, i, out, skipLeadingSpaces),

    local findNonSpaceToken(toks, i) =
      if toks[i].t == ' ' then i + 1
      else i,

    local parseTerm(toks, i0) =
      local i = findNonSpaceToken(toks, i0);
      local tok = toks[i];
      if tok.t == 'field' then
        [i + 1, { t: 'field', v: tok.v }]
      else if tok.t == 'var' then
        [i + 1, { t: 'var', v: tok.v }]
      else if tok.t == 'id' then
        [i + 1, { t: 'id', v: tok.v }]
      else if tok.t == 'number' then
        [i + 1, { t: 'number', v: tok.v }]
      else if tok.t == 'string' then
        [i + 1, { t: 'string', v: tok.v }]
      else if tok.t == 'true' then
        [i + 1, { t: 'bool', v: true }]
      else if tok.t == 'false' then
        [i + 1, { t: 'bool', v: false }]
      else if tok.t == '(' then
        local res = parseAction(toks, i + 1), j = res[0], node = res[1];
        assert node.t == 'action';
        assert node.v.t == 'pipeline';
        [j, node.v]
      else error ('parseTerm: unexpected token: %s' % [tok.t]),

    local parseOperand(toks, i) =
      local res = parseTerm(toks, i), j = res[0], node = res[1];
      if toks[j].t == 'field' then
        local
          aux(i, out) =
            if i >= std.length(toks) || toks[i].t != 'field' then out
            else aux(i + 1, out + [toks[i].v]),
          fields = aux(j, []);
        [j + std.length(fields), { t: 'chain', v: [node, fields] }]
      else [j, node],

    local parseCommand(toks, i) =
      local loop(i0, operands) =
        local i = findNonSpaceToken(toks, i0);
        if toks[i].t == '}}' || toks[i].t == ')' then
          [i, { t: 'command', v: operands }]
        else if toks[i].t == '|' then
          [i + 1, { t: 'command', v: operands }]
        else
          local res = parseOperand(toks, i), j = res[0], node = res[1];
          loop(j, operands + [node]);
      loop(i, []),

    local parsePipeline(toks, i0) =
      local loop(i0, commands) =
        local i = findNonSpaceToken(toks, i0);
        if toks[i].t == '}}' || toks[i].t == ')' then
          [i + 1, commands]
        else
          local res = parseCommand(toks, i), j = res[0], node = res[1];
          loop(j, commands + [node]);
      local i = findNonSpaceToken(toks, i0);
      if toks[i].t == 'var' &&
         (toks[i + 1].t == ':=' || toks[i + 1].t == '=')
      then
        local res = loop(i + 2, []), j = res[0], commands = res[1];
        [j, {
          t: 'pipeline',
          v: commands,
          d: [{ id: toks[i].v }],
          isa: toks[i + 1].t == '=',
        }]
      else if toks[i].t == 'var' && toks[i + 1].t == ' ' &&
              (toks[i + 2].t == ':=' || toks[i + 2].t == '=')
      then
        local res = loop(i + 3, []), j = res[0], commands = res[1];
        [j, {
          t: 'pipeline',
          v: commands,
          d: [{ id: toks[i].v }],
          isa: toks[i + 2].t == '=',
        }]
      else
        local res = loop(i, []), j = res[0], commands = res[1];
        [j, { t: 'pipeline', v: commands }],

    local parseControl(toks, i) =
      local res = parsePipeline(toks, i), j = res[0], pipe = res[1];
      local res = parseList(toks, j), k0 = res[0], list = res[1];
      local
        res =
          local k1 = findNonSpaceToken(toks, k0), k2 = findNonSpaceToken(toks, k1 + 1);
          if toks[k1].t == 'else' && toks[k2].t == '}}' then parseList(toks, k2 + 1)
          else [k0, null],
        l0 = res[0],
        elseList = res[1];
      local l1 = findNonSpaceToken(toks, l0), l2 = findNonSpaceToken(toks, l1 + 1);
      if toks[l1].t != 'end' || toks[l2].t != '}}' then
        error 'parseControl: end not found'
      else
        [l2 + 1, { pipe: pipe, list: list, elseList: elseList }],

    local parseAction(toks, i0) =
      local i = findNonSpaceToken(toks, i0);
      local tok = toks[i];
      if tok.t == 'with' || tok.t == 'if' || tok.t == 'range' then
        local res = parseControl(toks, i + 1), j = res[0], node = res[1];
        [j, { t: tok.t, v: node }]
      else if tok.t == 'else' || tok.t == 'end' then
        [i, null]
      else
        local res = parsePipeline(toks, i), j = res[0], node = res[1];
        [j, { t: 'action', v: node }],

    local parseList(toks, i) =
      local loop(i, root) =
        if i >= std.length(toks) then
          [i, root]
        else
          local tok = toks[i];
          if tok.t == 'text' then
            loop(i + 1, root { v+: [{ t: 'text', v: tok.v }] }) tailstrict
          else if tok.t == '{{' then
            local res = parseAction(toks, i + 1), j = res[0], node = res[1];
            if node == null then [j, root]
            else loop(j, root { v+: [node] }) tailstrict;
      loop(i, { t: 'list', v: [] }),

    local parse(toks/* tokens */, i) =
      local res = parseList(toks, i), j = res[0], node = res[1];
      if j < std.length(toks) then error 'parse: unexpected end'
      else node,

    local evalMark(s) =
      std.length(s.vars),

    local evalPop(s, mark) =
      s { vars: s.vars[std.length(s.vars) - mark:] },

    local evalPushVar(s, name, value) =
      s { vars: [{ name: name, value: value }] + s.vars },

    local evalSetVar(s, name, value) =
      local aux(i) =
        if i >= std.length(s.vars) then error ('evalSetVar: variable not found: ' + name)
        else if s.vars[i].name == name then
          s.vars[:i] + [{ name: name, value: value }] + s.vars[i + 1:]
        else
          aux(i + 1) tailstrict;
      s { vars: aux(0) },

    local evalGetVar(s, name) =
      std.filter(function(v) v.name == name, s.vars)[0].value,

    local evalFields(s, initialReceiver, fields) =
      std.foldl(
        function(receiver, field)
          if receiver == null then null
          else std.get(deref(s.h, receiver), field),
        fields,
        initialReceiver,
      ),

    local evalOperand(op, s0) =
      if op.t == 'chain' then
        local res = evalOperand(op.v[0], s0), s = res[0], val = res[1];
        [s, evalFields(s0, val, op.v[1])]
      else if op.t == 'field' then
        [s0, if op.v == '' then s0.dot else evalFields(s0, s0.dot, [op.v])]
      else if op.t == 'var' then
        [s0, evalGetVar(s0, op.v)]
      else if op.t == 'number' || op.t == 'string' || op.t == 'bool' then
        [s0, op.v]
      else if op.t == 'pipeline' then
        evalPipeline(op, s0)
      else if op.t == 'id' then
        // function call with no arguments
        if op.v == 'list' then
          local res = fromConst(s0.h, []), newheap = res[0], v = res[1];
          [s0 { h: newheap }, v]
        else error 'evalOperand: not implemented function'
      else
        error ('evalOperand: unknown operand: %s' % [op]),

    local predefinedFuncs = {
      indent(args): [indent(args.args), args.vs, args.h],
      nindent(args): [nindent(args.args), args.vs, args.h],
      toYaml(args): [toYaml([toConst(args.h, args.args[0])]), args.vs, args.h],
      printf(args): [printf(args.args), args.vs, args.h],
      and(args): [std.foldl(function(acc, x) acc && isTrueOnHeap(args.heap, x), args.args, true), args.vs, args.h],
      or(args): [std.foldl(function(acc, x) acc || isTrueOnHeap(args.heap, x), args.args, false), args.vs, args.h],
      default(args): default(args),
      ternary(args): [ternary(args.args), args.vs, args.h],
      replace(args): [replace(args.args), args.vs, args.h],
      b64dec(args): b64dec(args),
      not(args): not(args),
      empty(args): empty(args),
      index(args): index(args),
      append(args): append(args),
      join(args): join(args),
    },

    local evalCommand(command, final, s0) =
      local op0 = command.v[0];  // FIXME
      if op0.t == 'id' then
        if std.objectHas(predefinedFuncs, op0.v) then
          local
            res =
              std.foldl(
                function(acc, c)
                  local res = evalOperand(c, acc[0]), s = res[0], val = res[1];
                  [s, acc[1] + [val]],
                command.v[1:],
                [s0, []],
              ),
            s = res[0],
            args = res[1];
          local args1 = if final == null then args else args + [final];
          local res = predefinedFuncs[op0.v]({ h: s.h, args: args1, vs: 'no vs' });
          [s { h: res[2] }, res[0]]
        else if op0.v == 'template' || op0.v == 'include' then
          local res = evalOperand(command.v[1], s0), s1 = res[0], name = res[1];
          local res = evalOperand(command.v[2], s1), s2 = res[0], newDot = res[1];
          local res = include({ '$': templates, args: [name, newDot], vs: {}, h: s2.h });
          [s2 { h: res[2] }, res[0]]
        else if op0.v == 'tpl' then
          local res = evalOperand(command.v[1], s0), s1 = res[0], name = res[1];
          local res = evalOperand(command.v[2], s1), s2 = res[0], newDot = res[1];
          local res = tpl({ '$': templates, args: [name, newDot], vs: {}, h: s2.h });
          [s2 { h: res[2] }, res[0]]
        else
          error ('evalCommand: unknown id: %s' % [op0.v])
      else
        evalOperand(op0, s0),

    local evalPipeline(node, s0) =
      local commands = node.v;
      local decls = if std.objectHas(node, 'd') then node.d else null;
      local acc =
        std.foldl(
          function(acc, command)
            local s0 = acc.s, final = acc.final;
            local res = evalCommand(command, final, s0), s1 = res[0], v = res[1];
            { s: s1, final: v },
          commands,
          { s: s0, final: null },
        );
      local s = acc.s;
      local v = acc.final;
      if decls == null then [s, v]
      else
        local s1 =
          if node.isa
          then evalSetVar(acc.s, decls[0].id, v)
          else evalPushVar(acc.s, decls[0].id, v);
        [s1, ''],

    local eval(node, s0) =
      if node.t == 'text' then
        s0 { out+: node.v }
      else if node.t == 'list' then
        std.foldl(function(s, node) eval(node, s), node.v, s0)
      else if node.t == 'action' then
        assert node.v.t == 'pipeline';
        local res = evalPipeline(node.v, s0), s = res[0], val = res[1];
        if val == null then s else s { out+: std.toString(val) }
      else if node.t == 'with' || node.t == 'if' then
        local mark = evalMark(s0);
        local res = evalPipeline(node.v.pipe, s0), s = res[0], pipeVal = res[1];
        local finalState =
          if isTrueOnHeap(s0.h, pipeVal) then
            local s1 = eval(node.v.list, if node.t == 'if' then s else s { dot: pipeVal });
            s1 { dot: s.dot }
          else if node.v.elseList != null then
            eval(node.v.elseList, s) tailstrict
          else
            s0;
        evalPop(finalState, mark)
      else if node.t == 'range' then
        assert !std.objectHas(node.v.pipe, 'd') : 'not implemented';
        assert node.v.elseList == null : 'not implemented';
        local mark0 = evalMark(s0);
        local res = evalPipeline(node.v.pipe, s0), s1 = res[0], pipeVal = res[1];
        local oneInteration(s, val) =
          eval(node.v.list, s { dot: val });
        local finalState =
          if pipeVal == null then ''
          else
            local vals = deref(s1.h, pipeVal);
            if std.isArray(vals) then
              std.foldl(oneInteration, vals, s1) { dot: s1.dot }
            else if std.isObject(vals) then
              std.foldl(oneInteration, std.objectValues(vals), s1) { dot: s1.dot }
            else error 'eval: unexpected pipeline for range';
        evalPop(finalState, mark0)
      else error 'eval: unexpected node',

    strIndex: strIndex,
    findNonSpace: findNonSpace,
    lex: lex,
    parse: parse,
    eval: eval,
  },

      tpl(args0) =
  local templates = args0['$'], args = args0.args, vs = args0.vs, heap = args0.h;
  local tpl__ = tpl_(templates), src = args[0], dot = args[1];
  local evalResult =
    tpl__.eval(
      tpl__.parse(
        tpl__.lex(src, 0, []),
        0,
      ),
      {
        dot: dot,
        out: '',
        vars: [{ name: ''/* $ */, value: dot }],
        h: heap,
      },
    ).out;
  assert std.isString(evalResult);
  [evalResult, vs, heap];

local mergeTwoValues(heap, dstp, srcp) =
  if !isAddr(dstp) || !isAddr(srcp) ||
     !std.isObject(deref(heap, dstp)) || !std.isObject(deref(heap, srcp))
  then
    error 'mergeTwoValues: not object'
  else
    local src = deref(heap, srcp);
    local newheap = std.foldl(
      function(heap, key)
        local dst = deref(heap, dstp);
        if std.objectHas(dst, key) then
          if dst[key] == null then
            assign(heap, dstp, objectRemoveKey(dst, key))
          else if
            isAddr(dst[key]) && std.isObject(deref(heap, dst[key])) &&
            isAddr(src[key]) && std.isObject(deref(heap, src[key]))
          then
            local newheap = mergeTwoValues(heap, dst[key], src[key]);
            assign(newheap, dstp, dst)
          else
            heap
        else
          assign(heap, dstp, dst { [key]: src[key] }),
      std.objectFields(deref(heap, srcp)),
      heap,
    );
    newheap;

local parseKubeVersion(src) =
  local i0 = if src[0] == 'v' then 1 else 0;
  local res = std.split(src[i0:], '.');
  assert std.length(res) == 3;
  {
    Major: res[0],
    Minor: res[1],
    Version: 'v%s.%s.%s' % res,
    GitVersion: self.Version,
  };

local glob(heap, files, pattern) =
  // FIXME: implement
  local list =
    if pattern == 'files/dashboards/**.yaml' then
      {
        [fileName]: files[fileName]
        for fileName in std.filter(
          function(fileName)
            std.startsWith(fileName, 'files/dashboards/') &&
            std.endsWith(fileName, '.yaml'),
          std.objectFields(files),
        )
      }
    else if pattern == 'files/rules/**.yaml' then
      {
        [fileName]: files[fileName]
        for fileName in std.filter(
          function(fileName)
            std.startsWith(fileName, 'files/rules/') &&
            std.endsWith(fileName, '.yaml'),
          std.objectFields(files),
        )
      }
    else
      error ('glob: not implemented: "%s"' % pattern);
  fromConst(heap, list);

local
  chartMetadata(
    name,
    version,
    appVersion,
    templateBasePath,
    condition,
    renderedKeys,
    defaultValues,
    crds,
    files,
    subCharts,
  ) =
    {
      name: name,
      version: version,
      appVersion: appVersion,
      templateBasePath: templateBasePath,
      condition: condition,
      renderedKeys: renderedKeys,
      defaultValues: defaultValues,
      crds: crds,
      files: files,
      subCharts: subCharts,
    };

local constructValues(heap, values, meta, release, capabilities) =
  local mergeRecursively(heap, values, meta) =
    local heap1 = mergeTwoValues(heap, values, meta.defaultValues);
    local
      res = std.foldl(
        function(acc, meta)
          local heap = acc[0], subCharts = acc[1];
          local
            res =
              local objv = deref(heap, values);
              if std.objectHas(objv, meta.name) then
                [heap, objv[meta.name]]
              else
                local res = allocate(heap, {}), heap1 = res[0], addr = res[1];
                local newobjv = objv { [meta.name]: addr };
                local heap2 = assign(heap1, values, newobjv);
                [heap2, addr],
            heap1 = res[0],
            subValues = res[1];
          local
            res = mergeRecursively(heap1, subValues, meta),
            heap2 = res[0],
            dotp = res[1];
          [heap2, subCharts { [meta.name]: dotp }],
        meta.subCharts,
        [heap1, {}],
      ),
      heap2 = res[0],
      subCharts = res[1];
    local
      res = fromConst(heap2, {
        Chart: {
          Name: meta.name,
          Version: meta.version,
          AppVersion: meta.appVersion,
        },
        Release: release,
        Capabilities: capabilities,
        Files: {
          Get(heap, args):
            assert std.length(args) == 1;
            assert std.isString(args[0]);
            [heap, meta.files[args[0]]],
          Glob(heap, args):
            assert std.length(args) == 1;
            assert std.isString(args[0]);
            glob(heap, meta.files, args[0]),
        },

        // Filled later
        Values: {},
        Template: {},
        Subcharts: {},
      }),
      heap3 = res[0],
      dotp = res[1];
    local heap4 = assign(heap3, deref(heap3, dotp).Values, deref(heap3, values));
    local heap5 = assign(heap4, deref(heap4, dotp).Subcharts, subCharts);
    [heap5, dotp];
  mergeRecursively(heap, values, meta);

local doesConditionSatisfy(heap, condition, dotp) =
  if condition == '' then true
  else
    local fields = std.split(condition, '.');
    local values = deref(heap, dotp).Values;
    local result = std.foldl(
      function(val, field)
        if val == null then null
        else
          local derefedVal = deref(heap, val);
          if field in derefedVal then derefedVal[field] else null,
      fields,
      values,
    );
    result == true;

local renderChart(heap, templates, dotp, meta, release) =
  local heap2 = heap;
  local mainOutput =
    std.foldl(
      function(out, key)
        local
          heap3 = assign(
            heap2,
            deref(heap2, dotp).Template,
            { Name: key, BasePath: meta.templateBasePath },
          );
        out + [templates[key](heap3, dotp)[0]],
      meta.renderedKeys,
      [],
    );
  local subChartsOutput =
    std.map(
      function(subChart)
        if !doesConditionSatisfy(heap, subChart.condition, dotp) then []
        else
          local
            derefedDot = deref(heap, dotp),
            subDot = deref(heap, derefedDot.Subcharts)[subChart.name],
            derefedSubValues = deref(heap, deref(heap, subDot).Values),
            globalValues = deref(heap, derefedDot.Values).global;
          local res = allocate(heap, {}), heap1 = res[0], subValues = res[1];
          local heap2 = assign(heap1, subValues, derefedSubValues);
          local heap3 = _set(heap2, subValues, 'global', globalValues);
          local heap4 = _set(heap3, subDot, 'Values', subValues);
          renderChart(heap4, templates, subDot, subChart, release),
      meta.subCharts,
    );
  mainOutput + std.flattenArrays(subChartsOutput);

local flatten(ary) =
  local loop(i, out) =
    if i >= std.length(ary) then out
    else if std.isArray(ary[i]) then loop(i + 1, out + ary[i]) tailstrict
    else loop(i + 1, out + [ary[i]]) tailstrict;
  loop(0, []) tailstrict;

local parseManifests(src) =
  local manifests = std.join(
    '\n---\n',
    std.map(
      std.trim,
      std.split(
        if std.startsWith(src, '---') then src[3:] else src,
        '\n---',
      ),
    ),
  );
  local parsed = parseYaml(manifests);
  if parsed == null || std.isArray(parsed) then parsed
  else [parsed];

local chartMain(capabilities0, rootChartMetadata, initialHeap, templates) =
  function(values={}, namespace='default', includeCrds=false, kubeVersion='1.32.0', releaseName=rootChartMetadata.name)
    local values1 = values {
      global: if 'global' in super then super.global else {},
    };
    local res = fromConst(initialHeap, values1), heap1 = res[0], valuesp = res[1];
    local release = {
      Name: releaseName,
      Namespace: namespace,
      Service: 'Helm',
    };
    local capabilities = capabilities0 {
      KubeVersion: parseKubeVersion(kubeVersion),
      APIVersions: {  // FIXME: APIVersions should behave as an array, too.
        Has(heap, args):
          assert std.length(args) == 1;
          assert std.isString(args[0]);
          // FIXME: support resource name like "apps/v1/Deployment"
          [heap, std.member(capabilities0.APIVersions, args[0])],
      },
    };
    local
      res = constructValues(heap1, valuesp, rootChartMetadata, release, capabilities),
      heap2 = res[0],
      dotp = res[1];
    local renderedManifests = renderChart(
      heap2,
      templates,
      dotp,
      rootChartMetadata,
      release,
    );
    std.filter(
      function(x) x != null,
      std.flattenArrays(
        std.filter(
          function(x) x != null,
          std.map(parseManifests, renderedManifests),
        ),
      ),
    );

// DON'T USE BELOW

assert std.assertEqual(strIndex('{{', 'abc', 0), -1);
assert std.assertEqual(strIndex('{{', '{{c', 0), 0);
assert std.assertEqual(strIndex('{{', 'a{{', 0), 1);
assert std.assertEqual(strIndex('{{', 'a{{b{{', 3), 4);
assert std.assertEqual(strIndex('{{', 'a{{b{{', 5), -1);
assert std.assertEqual(strIndex('{{', '{{ with .A }}{{.B}}{{ end }}', 13), 13);

assert or({ args: [0, 0], vs: {}, heap: {} })[0] == 0;
assert or({ args: [1, 0], vs: {}, heap: {} })[0] == 1;
assert or({ args: [0, true], vs: {}, heap: {} })[0] == true;
assert or({ args: [1, 1], vs: {}, heap: {} })[0] == 1;

assert and({ args: [false, 0], vs: {}, heap: {} })[0] == false;
assert and({ args: [1, 0], vs: {}, heap: {} })[0] == 0;
assert and({ args: [0, true], vs: {}, heap: {} })[0] == 0;
assert and({ args: [1, 1], vs: {}, heap: {} })[0] == 1;

assert dir(['/run/topolvm/lvmd.sock']) == '/run/topolvm';

assert index({
  local input = fromConst({}, [0, [0, 0, [0, 0, 0, 1]]]),
  args: [input[1], 1, 2, 3],
  vs: {},
  h: input[0],
})[0] == 1;

assert trimAll(['ac', 'aabbcc']) == 'bb';

local tpl__ = tpl_({});
assert tpl__.findNonSpace(' a', 0, 1) == 1;
assert tpl__.findNonSpace('a ', 1, -1) == 0;
assert tpl__.findNonSpace(' ', 0, -1) == -1;
assert tpl__.findNonSpace(' ', 0, 1) == 1;
assert tpl__.lex('aa', 0, []) == [{ t: 'text', v: 'aa' }];
assert tpl__.lex('{{}}', 0, []) == [{ t: '{{' }, { t: '}}' }];
assert tpl__.lex('a{{}}', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: '}}' }];
assert tpl__.lex('a {{}}', 0, []) == [{ t: 'text', v: 'a ' }, { t: '{{' }, { t: '}}' }];
assert tpl__.lex('{{- }}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: '}}' }];
assert tpl__.lex('a{{- }}', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: ' ' }, { t: '}}' }];
assert tpl__.lex('a {{- }}', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: ' ' }, { t: '}}' }];
assert tpl__.lex('{{ -}}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: '}}' }];
assert tpl__.lex('{{ -}}a', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl__.lex('{{ -}} a', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl__.lex('{{- -}}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: '}}' }];
assert tpl__.lex('a{{- -}}a', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: ' ' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl__.lex('a {{- -}}a', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: ' ' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl__.lex('a{{- -}} a', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: ' ' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl__.lex('a {{- -}} a', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: ' ' }, { t: '}}' }, { t: 'text', v: 'a' }];
assert tpl__.lex('a{{}}b', 0, []) == [{ t: 'text', v: 'a' }, { t: '{{' }, { t: '}}' }, { t: 'text', v: 'b' }];
assert tpl__.lex('{{ . }}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: 'field', v: '' }, { t: ' ' }, { t: '}}' }];
assert tpl__.lex('{{ .A }}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: 'field', v: 'A' }, { t: ' ' }, { t: '}}' }];
assert tpl__.lex('{{ .A.b }}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: 'field', v: 'A' }, { t: 'field', v: 'b' }, { t: ' ' }, { t: '}}' }];
assert tpl__.lex('{{ .A.b }}', 0, []) == [{ t: '{{' }, { t: ' ' }, { t: 'field', v: 'A' }, { t: 'field', v: 'b' }, { t: ' ' }, { t: '}}' }];
assert tpl__.parse(tpl__.lex('', 0, []), 0) == { t: 'list', v: [] };
assert tpl__.parse(tpl__.lex('a', 0, []), 0) == { t: 'list', v: [{ t: 'text', v: 'a' }] };
assert tpl__.parse(tpl__.lex('a{{}}b', 0, []), 0) == {
  t: 'list',
  v: [
    { t: 'text', v: 'a' },
    { t: 'action', v: { t: 'pipeline', v: [] } },
    { t: 'text', v: 'b' },
  ],
};
assert tpl__.parse(tpl__.lex('a{{.}}b', 0, []), 0) == { t: 'list', v: [
  { t: 'text', v: 'a' },
  { t: 'action', v: { t: 'pipeline', v: [
    { t: 'command', v: [{ t: 'field', v: '' }] },
  ] } },
  { t: 'text', v: 'b' },
] };

local tpl___(args) =
  local res = fromConst({}, args[1]), heap = res[0], dot = res[1];
  tpl({
    '$': { tpl0(heap, dot): [deref(heap, dot).valueTpl0] },
    args: [args[0], dot],
    vs: {},
    h: heap,
  })[0];
assert tpl___(['', {}]) == '';
assert tpl___(['a', {}]) == 'a';
assert tpl___(['{', {}]) == '{';
assert tpl___(['{ {', {}]) == '{ {';
assert tpl___(['a{{}}b', {}]) == 'ab';
assert tpl___(['a{{.}}b', 3]) == 'a3b';
assert tpl___(['a{{.A}}b', { A: 3 }]) == 'a3b';
assert tpl___(['a{{.A.b}}b', { A: { b: 'c' } }]) == 'acb';
assert tpl___(['a{{.A.b}}{{.A.b}}b', { A: { b: 'c' } }]) == 'accb';
assert tpl___(['a{{.A.b | nindent 1}}b', { A: { b: 'c' } }]) == 'a\n cb';
assert tpl___(['a{{.A.b | nindent 1 | nindent 1}}b', { A: { b: 'c' } }]) == 'a\n \n  cb';
assert tpl___(['a{{$}}b', 3]) == 'a3b';
assert tpl___(['a{{$.A}}b', { A: 3 }]) == 'a3b';
assert tpl___(['a{{$.A.b}}b', { A: { b: 'c' } }]) == 'acb';
assert tpl___(['{{ include "tpl0" $ }}', { valueTpl0: 'here' }]) == 'here';
assert tpl___(['{{ include "tpl0" . }}', { valueTpl0: 'here' }]) == 'here';
assert tpl___(['>{{ with $ }}1{{ end }}<', true]) == '>1<';
assert tpl___(['>{{ with $ }}1{{ end }}<', false]) == '><';
assert tpl___(['{{ with .A }}{{.B}}{{ end }}', { A: { B: 1 } }]) == '1';
assert tpl___(['>{{ with $ }}1{{ else }}0{{ end }}<', true]) == '>1<';
assert tpl___(['>{{ with $ }}1{{ else }}0{{ end }}<', false]) == '>0<';
assert tpl___(['>{{ if $ }}1{{ end }}<', true]) == '>1<';
assert tpl___(['>{{ if $ }}1{{ end }}<', false]) == '><';
assert tpl___(['{{ if .A }}{{.B}}{{ end }}', { A: { B: 1 }, B: 0 }]) == '0';
assert tpl___(['>{{ if $ }}1{{ else }}0{{ end }}<', true]) == '>1<';
assert tpl___(['>{{ if $ }}1{{ else }}0{{ end }}<', false]) == '>0<';
assert tpl___(['{{ tpl "{{.A}}" $ }}', { A: 10 }]) == '10';
assert tpl___(['{{ tpl (toYaml .A) . }}', { A: { B: '{{.B}}' }, B: 'hello' }]) == 'B: "hello"';
assert tpl___(['{{ with .A }}{{ end }}{{ .C }}', { A: { B: 1 }, C: 2 }]) == '2';
assert tpl___(['{{`{{`}}', {}]) == '{{';
assert tpl___(['{{ $v := .A }}{{ $v }}', { A: 42 }]) == '42';
assert tpl___(['{{ $v := .A }}{{ $v }}{{ $v = .B }}{{ $v }}', { A: 42, B: 10 }]) == '4210';
assert tpl___(['{{ $v := .A }}{{ $v }}{{ if true }}{{ $v }}{{ $v := .B }}{{ $v }}{{ end }}{{ $v }}', { A: 0, B: 1 }]) == '0010';
assert tpl___(['{{ $v := .A }}{{ $v }}{{ if true }}{{ $v }}{{ $v = .B }}{{ $v }}{{ end }}{{ $v }}', { A: 0, B: 1 }]) == '0011';
assert tpl___(['{{ range .A }}{{ . }}{{ end }}', { A: [1, 2] }]) == '12';
assert tpl___(['{{ range .A }}{{ . }}{{ end }}', { A: { one: 1, two: 2 } }]) == '12';

assert fromConst({}, 10) == [{}, 10];
assert fromConst({}, true) == [{}, true];
assert fromConst({}, 'a') == [{}, 'a'];
assert
  local res = fromConst({}, function() 42), heap = res[0], v = res[1];
  deref(heap, v)() == 42;
assert
  local res = fromConst({}, [1]), heap = res[0], v = res[1];
  deref(heap, v)[0] == 1;
assert
  local res = fromConst({}, [0, [1]]), heap = res[0], v = res[1];
  deref(heap, deref(heap, v)[1])[0] == 1;
assert
  local res = fromConst({}, { a: 1 }), heap = res[0], v = res[1];
  deref(heap, v).a == 1;
assert
  local res = fromConst({}, { a: 0, b: { c: 1 } }), heap = res[0], v = res[1];
  deref(heap, deref(heap, v).b).c == 1;
assert
  local res = fromConst({}, { a: 0, b: [1] }), heap = res[0], v = res[1];
  deref(heap, deref(heap, v).b)[0] == 1;
assert local res = fromConst({}, 1); toConst(res[0], res[1]) == 1;
assert local res = fromConst({}, function() 42); toConst(res[0], res[1])() == 42;
assert local res = fromConst({}, [1, [2]]); toConst(res[0], res[1]) == [1, [2]];
assert local res = fromConst({}, { a: 0, b: [1] }); toConst(res[0], res[1]) == { a: 0, b: [1] };

local runMergeTwoValues(dst, src) =
  local heap0 = {};
  local res = fromConst(heap0, dst), heap1 = res[0], dstp = res[1];
  local res = fromConst(heap1, src), heap2 = res[0], srcp = res[1];
  local heap3 = mergeTwoValues(heap2, dstp, srcp);
  //  std.trace('%s %s %s\n%s' % [heap2, dst, src, mergeTwoValues(heap2, dst, src)], false);
  toConst(heap3, dstp);

assert runMergeTwoValues({}, {}) == {};
assert runMergeTwoValues({ a: 1 }, {}) == { a: 1 };
assert runMergeTwoValues({}, { a: 1 }) == { a: 1 };
assert runMergeTwoValues({ a: 1 }, { a: 1 }) == { a: 1 };
assert runMergeTwoValues({ a: 1 }, { a: 2 }) == { a: 1 };
assert runMergeTwoValues({ a: null }, { a: 2 }) == {};
assert runMergeTwoValues({ a: 1, b: 2 }, { a: 2 }) == { a: 1, b: 2 };
assert runMergeTwoValues({ a: 1, b: 2 }, { a: 2, c: 3 }) == { a: 1, b: 2, c: 3 };
assert runMergeTwoValues({ a: { b: 1 } }, { a: { b: 2 }, c: 3 }) == { a: { b: 1 }, c: 3 };
assert runMergeTwoValues({ a: [1] }, { a: [2] }) == { a: [1] };

assert std.assertEqual(ext_('/a/b/c/bar.css'), '.css');
assert std.assertEqual(ext_('/'), '');
assert std.assertEqual(ext_(''), '');

assert std.assertEqual(base_('/a/b'), 'b');
assert std.assertEqual(base_('/'), '/');
assert std.assertEqual(base_(''), '.');

'ok'
