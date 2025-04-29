local helmhammer = {
  field(receiver, fieldName, args):
    if std.isFunction(receiver[fieldName]) then receiver[fieldName](args)
    else receiver[fieldName],

  join(ary):
    std.join('', std.map(std.toString, ary)),

  isTrue(v):
    if v == null then false
    else if std.isArray(v) || std.isObject(v) || std.isString(v) then std.length(v) > 0
    else if std.isBoolean(v) then v
    else if std.isFunction(v) then v != null
    else if std.isNumber(v) then v != 0
    else true,

  range(state, values, fthen, felse):
    if values == null then felse(state)
    else if std.isNumber(values) then
      self.range(state, std.makeArray(values, function(x) x), fthen, felse)
    else if std.isArray(values) then
      if std.length(values) == 0 then felse(state)
      else
        std.foldl(
          function(acc, value)
            local postState = fthen(acc.state, acc.i, value);
            {
              i: acc.i + 1,
              state: {
                v: acc.state.v + postState.v,
                vs: postState.vs,
              },
            },
          values,
          {
            i: 0,
            state: state,
          },
        ).state
    else if std.isObject(values) then
      if std.length(values) == 0 then felse(state)
      else
        std.foldl(
          function(acc, kv)
            local postState = fthen(acc.state, kv.key, kv.value);
            {
              i: acc.i + 1,
              state: {
                v: acc.state.v + postState.v,
                vs: postState.vs,
              },
            },
          std.objectKeysValues(values),
          {
            i: 0,
            state: state,
          },
        ).state
    else error 'range: not implemented',

  printf(args):
    std.format(args[0], args[1:]),

  include(root):
    function(args)
      root[args[0]](args[1]),

  contains(args):
    std.findSubstr(args[0], args[1]) != [],

  default(args):
    local v = args[0];
    if
      v == null ||
      std.isNumber(v) && v == 0 ||
      std.isString(v) && v == '' ||
      std.isArray(v) && v == [] ||
      std.isObject(v) && v == {} ||
      std.isBoolean(v) && v == false
    then
      args[1]
    else
      v,

  trimSuffix(args):
    if std.endsWith(args[1], args[0]) then
      std.substr(args[1], 0, std.length(args[1]) - std.length(args[0]))
    else
      args[1],

  trunc(args):
    if args[0] >= 0 then
      std.substr(args[1], 0, args[0])
    else
      std.substr(args[1], std.length(args[1]) + args[0], -args[0]),

  nindent(args):
    '\n' + $.indent(args),

  indent(args):
    std.join(
      '\n',
      std.map(
        function(x) std.repeat(' ', args[0]) + x,
        std.split(args[1], '\n'),
      ),
    ),

  replace(args):
    std.strReplace(args[2], args[0], args[1]),

  quote(args):
    std.format('"%s"', std.strReplace(args[0], '"', '\\"')),

  squote(args):
    std.format("'%s'", std.strReplace(args[0], "'", "\\'")),

  not(args):
    !args[0],

  toYaml(args):
    std.manifestYamlDoc(args[0], quote_keys=false),

  tpl_:
    {
      strIndex(pat, str, start):
        // FIXME: slow
        local occurrences = std.findSubstr(pat, str[start:std.length(str)]);
        if occurrences == [] then -1 else start + occurrences[0],

      findNonSpace(str, i, step):
        local c = str[i];
        if i < 0 || i >= std.length(str) then
          i
        else if c == ' ' || c == '\n' || c == '\r' || c == '\t' then
          self.findNonSpace(str, i + step, step)
        else
          i,

      lexText(str, i0, out, skipLeadingSpaces):
        assert i0 < std.length(str) : 'lexText: unexpected eof';
        local i =
          if skipLeadingSpaces then self.findNonSpace(str, i0, 1)
          else i0;
        assert i < std.length(str) : 'lexText: unexpected eof';
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
        local j = self.strIndex('{{', str, i);
        if j == -1 then out + [{ t: 'text', v: str[i:] }]
        else
          assert j + 2 < std.length(str) : 'lexText: unexpected {{';
          if str[j + 2] == '-' then
            local k = self.findNonSpace(str, j - 1, -1);
            self.lexInsideAction(
              str,
              j + 3,
              if i >= k + 1 then out else out + [{ t: 'text', v: str[i:k + 1] }]
            ) tailstrict
          else
            self.lexInsideAction(
              str,
              j + 2,
              if i >= j then out else out + [{ t: 'text', v: str[i:j] }]
            ) tailstrict,

      isAlphanumeric(ch):
        local c = std.codepoint(ch);
        ch == '_' ||
        std.codepoint('a') <= c && c <= std.codepoint('z') ||
        std.codepoint('A') <= c && c <= std.codepoint('Z') ||
        std.codepoint('0') <= c && c <= std.codepoint('9'),

      // lexFieldOrVariable scans a field or variable: [.$]Alphanumeric.
      // The . or $ has been scanned.
      lexFieldOrVariable(str, i):
        local
          loop(i) =
            if i >= std.length(str) then error 'lexFieldOrVariable: unexpected eof'
            else if self.isAlphanumeric(str[i]) then loop(i + 1) tailstrict
            else i,
          j = loop(i);
        [j, str[i:j]],

      lexInsideAction(str, i, out):
        if i + 2 < std.length(str) && str[i] == '-' && str[i + 1] == '}' && str[i + 2] == '}' then
          self.lex(str, i + 3, out, skipLeadingSpaces=true)
        else if i + 1 < std.length(str) && str[i] == '}' && str[i + 1] == '}' then
          self.lex(str, i + 2, out)
        else
          local c = str[i];
          if c == '.' then
            local res = self.lexFieldOrVariable(str, i + 1), j = res[0], v = res[1];
            self.lexInsideAction(str, j, out + [{ t: 'field', v: v }]) tailstrict
          else if c == ' ' then
            self.lexInsideAction(str, i + 1, out) tailstrict
          else error 'lexInsideAction: unexpected char',

      lex(str, i, out, skipLeadingSpaces=false):
        if i >= std.length(str) then
          out
        else
          self.lexText(str, i, out, skipLeadingSpaces),

      parse(toks/* tokens */, i):
        local loop(i, root) =
          if i >= std.length(toks) then
            root
          else
            local tok = toks[i];
            if tok.t == 'text' then
              loop(i + 1, root { v+: [{ t: 'text', v: tok.v }] }) tailstrict;
        loop(i, { t: 'list', v: [] }),

      eval(node, dot, out):
        if node.t == 'text' then
          out + node.v
        else if node.t == 'list' then
          std.foldl(function(out, node) self.eval(node, dot, out), node.v, out),
    },

  tpl(args):
    local tpl_ = self.tpl_;
    tpl_.eval(tpl_.parse(tpl_.lex(args[0], 0, []), 0), args[1], ''),

  chartMain(
    chartName,
    chartVersion,
    chartAppVersion,
    releaseName,
    releaseService,
    keys,
    defaultValues,
    files,
  ):
    function(values={})
      local aux(key) =
        std.parseYaml(files[key]({
          Values: std.mergePatch(defaultValues, values),
          Chart: {
            Name: chartName,
            Version: chartVersion,
            AppVersion: chartAppVersion,
          },
          Release: {
            Name: releaseName,
            Service: releaseService,
          },
        }));
      std.filter(function(x) x != null, std.map(aux, keys)),
};
// DON'T USE BELOW

local tpl_ = helmhammer.tpl_;
assert tpl_.strIndex('', '', 0) == -1;
assert tpl_.strIndex('a', '', 0) == -1;
assert tpl_.strIndex('', 'a', 0) == -1;
assert tpl_.strIndex('a', 'a', 0) == 0;
assert tpl_.strIndex('b', 'a', 0) == -1;
assert tpl_.strIndex('a', 'a', 1) == -1;
assert tpl_.strIndex('a', 'aa', 1) == 1;
assert tpl_.strIndex('aa', 'baa', 1) == 1;
assert tpl_.findNonSpace(' a', 0, 1) == 1;
assert tpl_.findNonSpace('a ', 1, -1) == 0;
assert tpl_.findNonSpace(' ', 0, -1) == -1;
assert tpl_.findNonSpace(' ', 0, 1) == 1;
assert tpl_.lex('aa', 0, []) == [{ t: 'text', v: 'aa' }];
assert tpl_.lex('{{}}', 0, []) == [];
assert tpl_.lex('a{{}}', 0, []) == [{ t: 'text', v: 'a' }];
assert tpl_.lex('a {{}}', 0, []) == [{ t: 'text', v: 'a ' }];
assert tpl_.lex('{{- }}', 0, []) == [];
assert tpl_.lex('a{{- }}', 0, []) == [{ t: 'text', v: 'a' }];
assert tpl_.lex('a {{- }}', 0, []) == [{ t: 'text', v: 'a' }];
assert tpl_.lex('{{ -}}', 0, []) == [];
assert tpl_.lex('{{ -}}a', 0, []) == [{ t: 'text', v: 'a' }];
assert tpl_.lex('{{ -}} a', 0, []) == [{ t: 'text', v: 'a' }];
assert tpl_.lex('{{- -}}', 0, []) == [];
assert tpl_.lex('a{{- -}}a', 0, []) == [{ t: 'text', v: 'a' }, { t: 'text', v: 'a' }];
assert tpl_.lex('a {{- -}}a', 0, []) == [{ t: 'text', v: 'a' }, { t: 'text', v: 'a' }];
assert tpl_.lex('a{{- -}} a', 0, []) == [{ t: 'text', v: 'a' }, { t: 'text', v: 'a' }];
assert tpl_.lex('a {{- -}} a', 0, []) == [{ t: 'text', v: 'a' }, { t: 'text', v: 'a' }];
assert tpl_.lex('a{{}}b', 0, []) == [{ t: 'text', v: 'a' }, { t: 'text', v: 'b' }];
assert tpl_.lex('{{ . }}', 0, []) == [{ t: 'field', v: '' }];
assert tpl_.lex('{{ .A }}', 0, []) == [{ t: 'field', v: 'A' }];
assert tpl_.lex('{{ .A.b }}', 0, []) == [{ t: 'field', v: 'A' }, { t: 'field', v: 'b' }];
assert tpl_.lex('{{ .A.b }}', 0, []) == [{ t: 'field', v: 'A' }, { t: 'field', v: 'b' }];
assert tpl_.parse(tpl_.lex('', 0, []), 0) == { t: 'list', v: [] };
assert tpl_.parse(tpl_.lex('a', 0, []), 0) == { t: 'list', v: [{ t: 'text', v: 'a' }] };
assert tpl_.parse(tpl_.lex('a{{}}b', 0, []), 0) == { t: 'list', v: [{ t: 'text', v: 'a' }, { t: 'text', v: 'b' }] };

local tpl = helmhammer.tpl;
assert tpl(['', {}]) == '';
assert tpl(['a', {}]) == 'a';
assert tpl(['a{{}}b', {}]) == 'ab';

//helmhammer.tpl(['', {}]) == '' &&
//helmhammer.tpl(['abc', {}]) == 'abc' &&
//helmhammer.tpl(['{', {}]) == '{' &&
//helmhammer.tpl(['{ {', {}]) == '{ {' &&
//helmhammer.tpl(['{{.A}}', { A: 'hello' }]) == 'hello' &&
//helmhammer.tpl(['{{.A}}{{.A}}', { A: 'hello' }]) == 'hellohello' &&
//helmhammer.tpl(['{{.A.B}}', { A: { B: 'hello' } }]) == 'hello' &&
//helmhammer.tpl(['{{if .C}}{{.A.B}}{{end}}', { A: { B: 'hello' }, C: true }]) == 'hello' &&
//helmhammer.tpl(['{{if .C}}{{.A.B}}{{end}}', { A: { B: 'hello' }, C: false }]) == '' &&
//helmhammer.tpl([
//  '{{if .C}}{{.A.B}}{{else}}no{{end}}',
//  { A: { B: 'hello' }, C: false },
//]) == 'no' &&
'ok'
