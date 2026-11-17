# Sub-group Chain (Filteration)

- A new abstract algebraic structure for symbolic analysis of layout

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

## Stride Mapping (coordinate |-> offset space)

- Mapping the hardware coordinates obtained **to offset**

- Stride Vector is: ```(s[1][0], s[0][1], s[1][1])```

- Stride transformation
  - hardware_coordiantes dotproduct stride_vector

## Reshape Mapping (coordinate |-> coordinate)

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

- This is mostly a no-op
- The reason is
  - 1) During access mapping, we will put corresponding variable in the corresponding dimension
  - 2) Global memory access, cannot permutate dimension
    - Global coords to 1D offset mapping is fixed
    - Permutate other dimension in will be worse
  - 3) The only thing we can do is to allow more variable in the last dimension
    - Which we have already done during normal access mapping

- Notes: for f2, it is almost the same
  - We just fix cases that the degree of freedom is not enough, we swap in more binary/bit variable
