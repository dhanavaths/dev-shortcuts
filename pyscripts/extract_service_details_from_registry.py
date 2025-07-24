#!/usr/bin/env python3
"""
Extract service details from registry.yaml files and generate CSV output.

This script scans a directory structure looking for registry.yaml files in the format:
<namespace_dir>/<service_name_dir>/registry.yaml

It extracts specific fields and generates a CSV with columns:
namespace, servicename, instancessetting, CpuCores, MemoryInMB
"""

import os
import sys
import csv
import yaml
import logging
import argparse
from pathlib import Path
from typing import Dict, List, Optional, Tuple


def setup_logging(verbose: bool = False) -> None:
    """Setup logging configuration."""
    level = logging.DEBUG if verbose else logging.INFO
    logging.basicConfig(
        level=level,
        format='%(asctime)s - %(levelname)s - %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S'
    )


def find_registry_files(root_directory: Path) -> List[Tuple[str, str, Path]]:
    """
    Find all registry.yaml files in the expected directory structure.
    
    Args:
        root_directory: Root directory to search in
        
    Returns:
        List of tuples containing (namespace, service_name, registry_file_path)
    """
    registry_files = []
    
    if not root_directory.exists():
        logging.error(f"Root directory does not exist: {root_directory}")
        return registry_files
    
    logging.info(f"Scanning directory: {root_directory}")
    
    # Walk through namespace directories
    for namespace_dir in root_directory.iterdir():
        if not namespace_dir.is_dir():
            continue
            
        namespace = namespace_dir.name
        logging.debug(f"Found namespace directory: {namespace}")
        
        # Walk through service directories within namespace
        for service_dir in namespace_dir.iterdir():
            if not service_dir.is_dir():
                continue
                
            service_name = service_dir.name
            registry_file = service_dir / "registry.yaml"
            
            if registry_file.exists() and registry_file.is_file():
                registry_files.append((namespace, service_name, registry_file))
                logging.debug(f"Found registry.yaml: {namespace}/{service_name}")
            else:
                logging.debug(f"No registry.yaml found in: {namespace}/{service_name}")
    
    logging.info(f"Found {len(registry_files)} registry.yaml files")
    return registry_files


def parse_registry_yaml(file_path: Path) -> Optional[Dict]:
    """
    Parse a registry.yaml file and return the data.
    
    Args:
        file_path: Path to the registry.yaml file
        
    Returns:
        Parsed YAML data or None if parsing fails
    """
    try:
        with open(file_path, 'r', encoding='utf-8') as file:
            data = yaml.safe_load(file)
            return data
    except yaml.YAMLError as e:
        logging.error(f"YAML parsing error in {file_path}: {e}")
        return None
    except Exception as e:
        logging.error(f"Error reading file {file_path}: {e}")
        return None


def extract_service_details(namespace: str, service_name: str, yaml_data: Dict) -> Dict[str, str]:
    """
    Extract required service details from parsed YAML data.
    
    Args:
        namespace: Namespace name from directory structure
        service_name: Service name from directory structure
        yaml_data: Parsed YAML data
        
    Returns:
        Dictionary with extracted service details
    """
    details = {
        'namespace': namespace,
        'servicename': '',
        'instancessetting': '',
        'CpuCores': '',
        'MemoryInMB': ''
    }
    
    try:
        # Extract ServiceName from Metadata
        if 'Metadata' in yaml_data and 'ServiceName' in yaml_data['Metadata']:
            details['servicename'] = str(yaml_data['Metadata']['ServiceName'])
        else:
            details['servicename'] = service_name  # Fallback to directory name
            logging.warning(f"ServiceName not found in {namespace}/{service_name}, using directory name")
        
        # Extract Resource details
        if 'Resource' in yaml_data and 'Limits' in yaml_data['Resource']:
            limits = yaml_data['Resource']['Limits']
            
            # InstancesSetting
            if 'InstancesSetting' in limits:
                details['instancessetting'] = str(limits['InstancesSetting'])
            else:
                logging.warning(f"InstancesSetting not found in {namespace}/{service_name}")
            
            # CpuCores
            if 'CpuCores' in limits:
                details['CpuCores'] = str(limits['CpuCores'])
            else:
                logging.warning(f"CpuCores not found in {namespace}/{service_name}")
            
            # MemoryInMb
            if 'MemoryInMb' in limits:
                details['MemoryInMB'] = str(limits['MemoryInMb'])
            else:
                logging.warning(f"MemoryInMb not found in {namespace}/{service_name}")
        else:
            logging.warning(f"Resource.Limits section not found in {namespace}/{service_name}")
    
    except Exception as e:
        logging.error(f"Error extracting details from {namespace}/{service_name}: {e}")
    
    return details


def write_csv(service_details: List[Dict[str, str]], output_file: Path) -> None:
    """
    Write service details to CSV file.
    
    Args:
        service_details: List of service detail dictionaries
        output_file: Path to output CSV file
    """
    fieldnames = ['namespace', 'servicename', 'instancessetting', 'CpuCores', 'MemoryInMB']
    
    try:
        with open(output_file, 'w', newline='', encoding='utf-8') as csvfile:
            writer = csv.DictWriter(csvfile, fieldnames=fieldnames)
            writer.writeheader()
            writer.writerows(service_details)
        
        logging.info(f"CSV file written successfully: {output_file}")
        logging.info(f"Total records: {len(service_details)}")
    
    except Exception as e:
        logging.error(f"Error writing CSV file {output_file}: {e}")
        raise


def main():
    """Main function to orchestrate the extraction process."""
    parser = argparse.ArgumentParser(
        description='Extract service details from registry.yaml files and generate CSV',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python extract_service_details_from_registry.py
  python extract_service_details_from_registry.py -d /path/to/namespaces -o services.csv
  python extract_service_details_from_registry.py --verbose
        """
    )
    
    parser.add_argument(
        '-d', '--directory',
        type=Path,
        default=Path('.'),
        help='Root directory containing namespace directories (default: current directory)'
    )
    
    parser.add_argument(
        '-o', '--output',
        type=Path,
        default=Path('service_details.csv'),
        help='Output CSV file path (default: service_details.csv)'
    )
    
    parser.add_argument(
        '-v', '--verbose',
        action='store_true',
        help='Enable verbose logging'
    )
    
    args = parser.parse_args()
    
    # Setup logging
    setup_logging(args.verbose)
    
    logging.info("Starting service details extraction")
    logging.info(f"Input directory: {args.directory.absolute()}")
    logging.info(f"Output file: {args.output.absolute()}")
    
    try:
        # Find all registry.yaml files
        registry_files = find_registry_files(args.directory)
        
        if not registry_files:
            logging.warning("No registry.yaml files found")
            sys.exit(1)
        
        # Process each registry.yaml file
        service_details = []
        errors = 0
        
        for namespace, service_name, registry_file in registry_files:
            logging.debug(f"Processing: {namespace}/{service_name}")
            
            # Parse YAML file
            yaml_data = parse_registry_yaml(registry_file)
            if yaml_data is None:
                errors += 1
                continue
            
            # Extract service details
            details = extract_service_details(namespace, service_name, yaml_data)
            service_details.append(details)
        
        # Write CSV output
        if service_details:
            write_csv(service_details, args.output)
            
            logging.info(f"Successfully processed {len(service_details)} services")
            if errors > 0:
                logging.warning(f"Encountered {errors} errors during processing")
        else:
            logging.error("No valid service details extracted")
            sys.exit(1)
    
    except KeyboardInterrupt:
        logging.info("Process interrupted by user")
        sys.exit(1)
    except Exception as e:
        logging.error(f"Unexpected error: {e}")
        sys.exit(1)
    
    logging.info("Service details extraction completed successfully")


if __name__ == '__main__':
    main()