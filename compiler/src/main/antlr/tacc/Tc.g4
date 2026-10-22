/*
 * ANTLR 4 grammar of the .tc language.
 *
 * Parser rules follow spec/language.md -> "Grammar" rule by rule; the EBNF in the spec stays
 * authoritative. One deliberate difference: `equality` and `relational` accept a chain of
 * operators, so that AstBuilder can reject `a < b < c` with a clear message instead of a generic
 * syntax error.
 *
 * Lexer rules BAD_INT, UNTERMINATED_STRING and ERROR_CHAR never reach the parser: Parse.kt reports
 * them as lexical errors.
 */
grammar Tc;

// ---- Parser rules ----

program    : function* EOF ;
function   : FUN IDENT LPAREN (param (COMMA param)*)? RPAREN (COLON typeName)? block ;
param      : IDENT COLON typeName ;
typeName   : INT_TYPE | BOOL_TYPE | STRING_TYPE ;

block      : LBRACE statement* RBRACE ;
statement  : varDecl | assignment | ifStmt | whileStmt | returnStmt | callStmt ;
varDecl    : kind=(LET | CONST) IDENT (COLON typeName)? ASSIGN expression SEMI ;
assignment : IDENT ASSIGN expression SEMI ;
ifStmt     : IF LPAREN expression RPAREN block (ELSE (ifStmt | block))? ;
whileStmt  : WHILE LPAREN expression RPAREN block ;
returnStmt : RETURN expression? SEMI ;
callStmt   : call SEMI ;

expression : equality ;
equality   : relational (op+=(EQ | NE) relational)* ;
relational : additive (op+=(LT | LE | GT | GE) additive)* ;
additive   : term (op+=(PLUS | MINUS) term)* ;
term       : unary (op+=(STAR | SLASH) unary)* ;
unary      : op=(MINUS | BANG) unary
           | primary
           ;
primary    : INT                      # intLiteral
           | STRING                   # stringLiteral
           | (TRUE | FALSE)           # boolLiteral
           | call                     # callExpr
           | IDENT                    # varRef
           | LPAREN expression RPAREN # parens
           ;
call       : IDENT (DOT IDENT)? LPAREN (expression (COMMA expression)*)? RPAREN ;

// ---- Lexer rules ----

FUN         : 'fun' ;
LET         : 'let' ;
CONST       : 'const' ;
IF          : 'if' ;
ELSE        : 'else' ;
WHILE       : 'while' ;
RETURN      : 'return' ;
TRUE        : 'true' ;
FALSE       : 'false' ;
INT_TYPE    : 'Int' ;
BOOL_TYPE   : 'Bool' ;
STRING_TYPE : 'String' ;

LPAREN : '(' ;
RPAREN : ')' ;
LBRACE : '{' ;
RBRACE : '}' ;
DOT    : '.' ;
COMMA  : ',' ;
SEMI   : ';' ;
COLON  : ':' ;

EQ     : '==' ;
NE     : '!=' ;
LE     : '<=' ;
GE     : '>=' ;
LT     : '<' ;
GT     : '>' ;
ASSIGN : '=' ;
PLUS   : '+' ;
MINUS  : '-' ;
STAR   : '*' ;
SLASH  : '/' ;
BANG   : '!' ;

// Keywords above take precedence over IDENT for the same text; `letter` is still an IDENT.
IDENT  : [a-zA-Z_] [a-zA-Z0-9_]* ;
INT    : [0-9]+ ;
// Escapes are validated (only \" \\ \n) when the token is read, see Parse.kt.
STRING : '"' ( ~["\\\r\n] | '\\' ~[\r\n] )* '"' ;

COMMENT : '//' ~[\r\n]* -> skip ;
WS      : [ \t\r\n]+ -> skip ;

// Error tokens, reported as lexical errors by Parse.kt.
BAD_INT             : [0-9]+ [a-zA-Z_] [a-zA-Z0-9_]* ;
UNTERMINATED_STRING : '"' ( ~["\\\r\n] | '\\' ~[\r\n] )* ;
ERROR_CHAR          : . ;
