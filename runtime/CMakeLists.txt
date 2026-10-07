# Runtime is linked into the VM executable (docs/ai-context/01-architecture.md).
# It has no sources yet; change to a STATIC library when the first source file is added.
add_library(tacos_runtime INTERFACE)
