#!/usr/bin/env python3
"""
Automatically convert integration tests to multi-version format.
Usage: python3 convert_tests.py <test_file>
"""

import sys
import re

def convert_test_function(content):
    """Convert a single-version test to multi-version format"""
    
    # Pattern to match test function start
    func_pattern = r'(func (Test\w+)\(t \*testing\.T\) \{)'
    
    def replace_func(match):
        func_decl = match.group(1)
        func_name = match.group(2)
        
        # Return multi-version wrapper
        return f"""{func_decl}
\tfor _, tt := range standardVersionTests {{
\t\tt.Run(tt.name, func(t *testing.T) {{"""
    
    # Replace function declarations
    content = re.sub(func_pattern, replace_func, content)
    
    # Replace hardcoded versions
    content = re.sub(r'serverCfg\.Version = "1\.21\.\d+"', 'serverCfg.Version = tt.mcVersion', content)
    
    # Find agent config creation and add version handler setup
    agent_cfg_pattern = r'(agentCfg := DefaultAgentConfig\([^)]+\))\n\t(agentCfg\.EnableReplay)'
    
    version_handler_code = r'''\1
\t
\t// Setup version handler
\tif tt.useVersionHandler {
\t\tversionHandler, err := common.GetVersionHandler(tt.mcVersion)
\t\tif err == nil && versionHandler != nil {
\t\t\tagentCfg.VersionHandler = versionHandler
\t\t}
\t} else {
\t\tagentCfg.DisableVersionHandlerAutoDetect = true
\t}
\t
\t\2'''
    
    content = re.sub(agent_cfg_pattern, version_handler_code, content)
    
    # Close the t.Run and for loop at end of functions
    # This is tricky - need to find function ends
    
    return content

def add_import(content):
    """Add common import if not present"""
    if 'versions/common' not in content:
        import_pattern = r'(import \(\n[^)]+)'
        content = re.sub(
            import_pattern,
            r'\1\t"github.com/reallyoldfogie/mc-agent/versions/common"\n',
            content
        )
    return content

def main():
    if len(sys.argv) < 2:
        print("Usage: python3 convert_tests.py <test_file>")
        sys.exit(1)
    
    filepath = sys.argv[1]
    
    with open(filepath, 'r') as f:
        content = f.read()
    
    # Add import
    content = add_import(content)
    
    # Convert functions
    content = convert_test_function(content)
    
    # Write back
    with open(filepath, 'w') as f:
        f.write(content)
    
    print(f"Converted {filepath}")

if __name__ == '__main__':
    main()
