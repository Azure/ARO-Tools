#!/usr/bin/env python3
"""
Restructure Kubernetes YAML resources.

This script converts Helm's resource info format (where resources are organized by kind)
into standard multi-document YAML format with individual resources.

Usage:
    python restructure_resources_yaml.py <input_file> <output_file>
    
Example:
    python restructure_resources_yaml.py resources_info_example.yaml resources_restructured.yaml
"""

import sys
import yaml
import re


def restructure_resources_yaml(input_file, output_file):
    """
    Restructure Helm resource info format to standard multi-document YAML.
    
    Args:
        input_file: Path to input YAML file with resources organized by kind
        output_file: Path to output file for restructured YAML
    """
    # Load the grouped YAML
    with open(input_file, 'r') as f:
        data = yaml.safe_load(f)
    
    resources = []
    
    # Iterate through each kind grouping (e.g., "v1/Deployment", "v1/Pod(related)")
    for kind_key, kind_data in data.items():
        print(f"Processing kind: {kind_key}")
        
        if kind_data is None:
            continue
            
        # Handle different structures
        if isinstance(kind_data, list):
            # Direct list of resources
            for resource in kind_data:
                if isinstance(resource, dict):
                    # Check if this is a List type (PodList, etc.)
                    if 'items' in resource and isinstance(resource.get('items'), list):
                        # Extract individual items from the list
                        for item in resource['items']:
                            if isinstance(item, dict) and 'kind' in item:
                                resources.append(item)
                                print(f"  Added {item.get('kind', 'Unknown')}: {item.get('metadata', {}).get('name', 'Unknown')}")
                    elif 'kind' in resource:
                        # Regular resource
                        resources.append(resource)
                        print(f"  Added {resource.get('kind', 'Unknown')}: {resource.get('metadata', {}).get('name', 'Unknown')}")
        elif isinstance(kind_data, dict):
            # Single resource or List type
            if 'items' in kind_data and isinstance(kind_data.get('items'), list):
                # Extract individual items from the list
                for item in kind_data['items']:
                    if isinstance(item, dict) and 'kind' in item:
                        resources.append(item)
                        print(f"  Added {item.get('kind', 'Unknown')}: {item.get('metadata', {}).get('name', 'Unknown')}")
            elif 'kind' in kind_data:
                # Single resource
                resources.append(kind_data)
                print(f"  Added {kind_data.get('kind', 'Unknown')}: {kind_data.get('metadata', {}).get('name', 'Unknown')}")
    
    # Write out as multi-document YAML
    with open(output_file, 'w') as f:
        for i, resource in enumerate(resources):
            if i > 0:
                f.write('---\n')
                yaml.dump(resource, f, default_flow_style=False, sort_keys=False)
    
    print(f"\nRestructured {len(resources)} resources from {input_file} to {output_file}")



if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python restructure_resources_yaml.py <input_file> <output_file>")
        print("\nExample:")
        print("  python restructure_resources_yaml.py resources_info_example.yaml resources_restructured.yaml")
        sys.exit(1)
    
    input_file = sys.argv[1]
    output_file = sys.argv[2]
    restructure_resources_yaml(input_file, output_file)
