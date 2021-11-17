
# usage:
# file hierarchy:
#   your_module/
#     > cd to here, run: sh ../slime/bin/<this_script>
#   slime/bin/<this_script>
make -f "$(dirname $0)/../Makefile" generate
