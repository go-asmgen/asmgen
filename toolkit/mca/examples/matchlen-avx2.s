vmovdqu (%rax,%rdi), %ymm0
vmovdqu (%rbx,%rdi), %ymm1
vpcmpeqb %ymm1, %ymm0, %ymm0
vpmovmskb %ymm0, %r9d
cmpl $-1, %r9d
addq $32, %rdi
