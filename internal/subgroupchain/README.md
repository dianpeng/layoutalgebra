# Sub-group Chain (Filteration)

- A new abstract algebraic structure for symbolic analysis of layout

## Status (2026-11-17)

- Working
  - Expression IR: Variable / Const / Symbol / Binary(Add/Sub/Mul/Div/Mod) / Stride
    - Notes, `Div` is truncating integer division; `Stride` is an opaque dot-product
      node (coordinate . stride) generated at known callsites, with compile-known shape
  - Range analysis: monotone interval lattice (post-order data flow over the expr tree)
    - Variable -> [0, upper bound); Stride -> [0, rank); Const/Symbol -> Unknown
    - Div -> [0, ceil(U/d)); Mod -> [0, min(m, U)); Mul/Add and friends -> Unknown
  - `Factory.Factorize`: mixed-radix decomposition of a ranged variable
  - Tuple / ConstTuple / HTuple / ConstHTuple machinery (flatten, clone, rank, ...)
  - Access mapping (BoV |-> coordinate HTuple), incl. recursive (nested) tile shapes
  - Stride mapping (coordinate HTuple |-> byte offset), dtype-size scaling

- Verified
  - Soundness: Access + StrideMap compose to a **bijection** onto [0, N); enforced
    invariants: BoV product == tile rank, every dimension fully covered, every
    variable fully consumed, coordinate/shape structural match. Violations are
    loud errors, never silent wrong mappings
  - Exhaustive property test: all 1400 (BoV, tile-shape) pairs with equal product
    N <= 60 succeed and map bijectively (full assignment-space enumeration)

- Implemented conventions (diverging from the illustration below, read before use)
  - The access map peels the **low digit** of each variable per dimension
    (coordinate digit = v % g, carried remainder = v / g). The example below peels
    warp's high digit first for dim 0; both are valid bijections, the
    implementation is consistently low-digit
  - BoV order is **significant**: it is major -> minor significance. Within a
    dimension the first-consumed variable takes the most-significant digit; across
    dimensions, left is major. Put the lane variable last for GMEM coalescing
  - Trivial digits (bound 1) are skipped; extent-1 dimensions produce an empty
    (rank-1) Stride node

- Not yet implemented (planned)
  - Swizzle (SMem bank-conflict avoidance) and its transvection machinery
  - Folding / simplification engine (like f2's); expressions are kept as-is
  - Reshape mapping (merge / break) as explicit transformation passes
  - GMEM check diagnostic (lane-placement legality verdict, see GMem section)
  - No padding / truncation / injective-surjective support, by design

- Overview
  - Given a single induction variable as index of hardware concurrency
    - For cuda, it could be concate(blockIdx, threadIdx)
      - Mostly, compiler will ONLY work within CTA, so blockIdx is not needed
    - Denoted hc_induction_variable

  - Hardware coordinates are
    - given a dimension extent as (n[0], n[1], ..., n[j-1]) PI(n[0], ..., n[j-1]) = M
    - (h[0], h[1], ..., h[k-1])
      - h[i] = hc_induction_variable / s[i]
      - s.t PI(s[i]) = M
    - A series quotients sub-group
      - Where i'th sub-group is i-1'th sub-group's sub-group

    - For example:
      - 32 * 96, can be tiled with HTuple representation
        - ((4, 8), (16, 6))
          - ((s[0][0], s[1][0]), (s[0][1], s[1][1]))
        - Assuming the level of concurrency, ie # toal threads, is 32 * 96
          - We need to find induction variable accordingly
          - ((x0, y0), (x1, y1))
        - Assume threadIdx is thread index
          - x0 = threadIdx                            / PI(s[1][0], s[0][1], s[1][1])
          - y0 = (x0 % PI(s[1][0], s[0][1], s[1][1])) / PI(s[0][1], s[1][1]))
          - x1 = (y0 % PI(s[0][1], s[1][1]))          / PI(s[1][1])
          - y1 = (x1 % PI(s[1][1]))                   / 1

- Notes, this is used by CuTE layout
  - CuTE represents coordinate as a **HTuple**
    - A denormalized/recursive Tuple
    - A Tuple is our **Vector**
  - In our codebase, we will use
    - HVector => HTuple
    - Vector  => Tuple
  - Notes, in representation we will have
    - Shape        HTuple
    - Coordinate   HTuple
    - Stride       Tuple

# Overall

- The Sub-group chain is not as simple as F2
  - But it supports more shape, instead of only power of 2

- Certain operation requires using F2 prespective to work with

- Notes, we do not allow padding/truncation
  - No mismatched shape
  - No supporting of injective and surjective
  - Those shape alignment can be done upfront mostly
    - Each algebra can expose its constraints, which can be resolved upfront by compiler

# Transformation

- About representing the transformation as matrix
  - Notes matrix is just a way to perform transformation
    - For example, transformation derivation requires matrix equation solving
  - It is not always needed
  - For this types of representation, transformation matrix is not needed mostly
    - We can directly work on symbolic vectors

## Vocabulary

- Concurrency space
  - 1D induction variable of thread index
    - Upper bound is N (# of threads totally)
    - Conceptually, we will use BoV
  - Brag of Variables (BoV)
    - Our analysis needs special semantic attached to a variable
      - ie lane id variable for GMEM/SMEM optimization
    - To address this requirements, we allow user to define concurrency space as a
      - 1) Brag of Variables: ie a set of variables, unordered
      - 2) Each variable contains its own bounds
      - 3) For example, on CUDA
        - BoV: (warp_varaible, lane_variable)
        - warp_variable: warp index, [0, 32) (CTA maximum allowed threads)
        - lane_variable: lane index, [0, 32) (each WARP must have 32 threads)
    - Notes, the BoV is mapped from platform specific setup
      - For example, on CUDA
        - warp_variable = threadIdx >> 5
        - lane_variable = threadIdx & 31

- Coordinate space
  - (x, y, ..., z) that is compatible against with a tensor shape defined
  - Mapped from concurrency space, represented as BoV
  - After different transformation
  - Each coordinate instance will be bounded to its perspective shape
    - Same as CuTE layout
  - The very first coordinate space will be user provided tile size
    - Our job is to find out a mapping from BoV to tile size coordinate space
    - This mapping is called Access

- Offset space
  - Using stride mapping to map coordinate back to offset space
    - 1D scalar
  - Used for memory access

## Access (coordinate |-> coordinate)

- Representing how BoV mapped to each data tile space

- Status: implemented, see "Implemented conventions" above for the exact semantics

- Notes on the consumption algorithm (as implemented)
  - Dimensions are processed left to right; within a dimension, variables are
    scanned left to right and each contributes g = gcd(bound, remaining extent)
  - g == 1 means the variable shares no factor with the remaining extent: skipped
  - The variable's low digit (bound g) joins the dimension's Stride coordinate;
    the high part (bound/g) is carried to subsequent dimensions
  - The dimension is complete when its extent is reduced to 1; the mapping is
    accepted only if every dimension is covered and every variable is consumed
    (guaranteed to succeed whenever BoV product == tile rank)

- For example:
  - BoV: (warp, lane)
    - range is: 0 <= wrap < 14; 0 <= lane < 32
  - Tile shape is `[7][16][4]`
  - THe variable level breaking down will be
    - (14, 32) => ((7, 2), 32) => (7, 2 * 8, 4)
    - Plan: 
      - warp splitted into (7, 2)
        - warp0 = (warp % 14) / 2 => notes warp % 14 = warp, 14 is upper bound of warp
        - warp1 = (warp % 2)  / 1
      - lane splitted into (8, 4)
        - lane0 = (lane % 32) / 4 => notes lane % 32 = lane, 32 is upper bound of lane
        - lane1 = (lane % 4)  / 1

  - A access map will be: 
    - `vector(warp0, (warp1, lane0), lane1) = vector(warp0, warp1 * 8 + lane0, lane1)`
    - Notes to merge (warp1, lane0) we need to use warp1 * bound(lane0) + lane0
  - As implemented (low-digit peeling), the same inputs instead give the equally
    valid bijection
    - dim 0 (extent 7):  warp % 7
    - dim 1 (extent 16): ((warp / 7) % 2) * 8 + (lane % 8)
    - dim 2 (extent 4):  lane / 8

## Stride Mapping (coordinate |-> offset space)

- Mapping the hardware coordinates obtained **to offset**

- Status: implemented
  - Recurses through nested (htuple) shapes, mirroring the shape structure
  - Returns a byte-oriented offset: element offset * dtype size
  - Validates dtype size (1/2/4) and coordinate/shape structural match; mismatches
    are errors, not panics
  - Row-major strides derived from the shape via `common.Stride`

- Stride Vector is: ```(s[1][0], s[0][1], s[1][1])```

- Stride transformation
  - hardware_coordiantes dotproduct stride_vector

## Reshape Mapping (coordinate |-> coordinate)

- Status: NOT implemented (design below is aspirational)

- A specific shape to be rescheduled to another desired shape

- Why this is useful
  - 1) For a tile, assuming we want to load it from GMEM to ensure optimality
  - 2) Due to invalid mapping, the hardawre coordinates corresponds to **lane** are in higher order dimension
    - For example: 
      - the tile is [n0][n1]..[nk-1], the lane coords are responsible for n0/n1
      - this will for sure result in poor GMEM coalesce
  - 3) To allow optimality, we need to
    - Permutate the n0/n1 into lower order dimension
    - Reshape them if needed
      - Assume n0 + n1 contains both lane id variable + warp id variable
      - The GMEM optimization is only within the scope of warp
      - We need to facter out the warp id parts to make it ONLY on lane id variable

- Notes all shape change CAN ONLY BE in same dimension
  - Assuming upper layer gives us a tile/tensor with specific dimension
  - The extent for each dimension MUST BE assumed to be fixed
  - Reshape cross dimension is only working when
    - 1) UNDERLYING data is 1D, which is not always true
    - 2) OTHERWISE data is corrupted
  - In CuTE Layout representation
    - Reshape happened vertically for a specific dimension's all involved components

### Merge

- Merge 2 adjacent dimension together
  - Mutliple merges can be done recursively
  - None-adjacent dimension can be down with a permutation transformation before

- Multiply 2 adjacent dimension's extents to get new per dimension shape extent after merging

### Break

- Uses CuTE Layout
  - A dimension extent K can be recursively breaking down as (k[0], ..., k[j-1])
    - s.t PI(k[0], ..., k[j-1]) = K
    - Using **Hardware Coordinate Mapping** recursively here
    - Notes, vector is not a **flatten one**, but a *HTuple*

## SMem Bank Conflicts Avoidance

- Status: NOT implemented (depends on the swizzle machinery, which is also not
  implemented yet); design below

- Bank # model:
  - Assume a offset is byte oriented
  - offset >> **visting width**
    - visiting width on CUDA is 2, ie 4 bytes alignment
  - floor(offset / transaction_width)
    - CUDA, transaction_width is 32

- (0) Permutate
  - (1) The relevant dimension for bank conflicts are **last dimension** with given tile
    - Assuming it has extent M
  - (2) Meff = M / 2 ^ **visiting width**
  - (3) Mgoal = min(32, Meff)
    - At most 32 can be optimized for swizzle bank
  - (4) If Meff >= 32/Mgoal
    - The tile's last dimension is large enough to avoid swizzle directly
    - Check if Meff is covered by **lane variable parts**
      - If so, do nothing, already optimal
  - (5) Otherwise, we will have to do swizzle
    - The lane id covered variation is not fully within the effective parts
    - We need to draw them in the effective parts by **transvection**
    - (5.1) Finding all lane id variables mapping dimension
      - A sub HTuple
      - Along with them, get each variable's bound
    - (5.2) Facter out portion in Wgoal has been covered by lane id
      - (Wgoal.0, Wgoal.1): Assuming Wgoal.1 is covered
      - Bounds of Wgoal.1 is X
    - (5.2) Finding lane id covered portion s.t PI(...) >= X
    - (5.3) Swizzle is
      - (Wgoal.1 + coeff * W.lane) MOD X
        - Or W.lane mask out the needed bits for lane id
        - For power of 2 lane arrange using shift/mask
        - Otherwise just use MOD <lane bound>
      - coeff mostly just be 1 will be fine
      - shear = (current + S * R) mod X

## GMem Coalesce (coordiante |-> coordinate)

- Status: no optimizer implemented, and per the analysis below none is needed —
  what remains is a legality **check** (planned, not implemented)

- Why there is nothing to optimize
  - 1) During access mapping, we put corresponding variable in the corresponding dimension
  - 2) Global memory access cannot permutate dimension
    - Global coords to 1D offset mapping is fixed
    - Permutate other dimension in will be worse
  - 3) The only thing we can do is to allow more variable in the last dimension
    - Which we have already done during normal access mapping
  - 4) Corollary: given a fixed tile shape (user input, global strides are fixed by
     the problem) and a fixed opinioned access map, the GMEM access pattern is fully
     determined by the **BoV ordering** — the only remaining degree of freedom
    - BoV order is major -> minor significance (see "Implemented conventions"):
      put the lane variable last and it lands on the minor, unit-stride digits
    - A mis-ordered BoV still yields a valid bijection, just an uncoalesced one,
      so the framework must diagnose it, not silently accept it

- Planned check (no transformation, verdict only)
  - Walk the composed offset expression's mixed-radix digits; verify the lane
    variable's digits occupy the trailing tail with unit element strides (after
    dtype scaling) and cover PI(trailing extents) >= lane bound
  - Returns {Optimal, LaneMisplaced}; the user fixes it by reordering the BoV (or,
    upstream, by choosing a tile shape whose trailing extents factor against the
    lane bound)

- Notes: for f2, it is almost the same
  - We just fix cases that the degree of freedom is not enough, we swap in more binary/bit variable
  - In subgroupchain the analogous freedom is the user's BoV order + tile choice,
    both upstream of this algebra
