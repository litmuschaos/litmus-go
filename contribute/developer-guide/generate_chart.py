from jinja2 import Environment, FileSystemLoader, select_autoescape
import yaml
import os
import sys
import argparse

'''
TODO: 
  1. No try-catch blocks have been used to account for empty/uninitialized params in attributes.yaml

  2. Add basic unit/bdd tests (manually checked currently) & necessary code refactor to facilitate this: 

     a. (positive) Verify creation of CSV for --generate_type=chart
     b. (positive) Verify creation of CSV using category name for --generate_type=experiment
     c. (positive) Verify creation of business logic, job & experiment CRs for --generate_type=experiment
     d. (negative) Verify validation/err-handle upon empty name, category attributes
'''

'''
NOTES: 
  1. Category attribute is expected to match with chart names(though not mandatory), as per convention 
     followed in litmuschaos/chaos-charts
'''


# generate_csv creates the chartserviceversion manifest
def generate_csv(csv_parent_path, csv_name, csv_config, litmus_env):
    csv_filename = csv_parent_path + '/' + csv_name + '.' + 'chartserviceversion.yaml'

    # Load Jinja2 template
    template = litmus_env.get_template('./templates/chartserviceversion.tmpl')
    output_from_parsed_template = template.render(csv_config)
    with open(csv_filename, "w+") as f:
        f.write(output_from_parsed_template)

# generate_chart creates the experiment-custom-resource manifest
def generate_chart(chart_parent_path, chart_config, litmus_env):
    chart_filename = chart_parent_path + '/' + 'experiment.yaml'

    # Load Jinja2 template
    template = litmus_env.get_template('./templates/experiment_custom_resource.tmpl')
    output_from_parsed_template = template.render(chart_config)
    with open(chart_filename, "w+") as f:
        f.write(output_from_parsed_template)

# generate_rbac creates the rbac for the experiment
def generate_rbac(chart_parent_path, chart_config, litmus_env):
    rbac_filename = chart_parent_path + '/' + 'rbac.yaml'

    # Load Jinja2 template
    template = litmus_env.get_template('./templates/experiment_rbac.tmpl')
    output_from_parsed_template = template.render(chart_config)
    with open(rbac_filename, "w+") as f:
        f.write(output_from_parsed_template)

# generate_engine creates the chaos engine for the experiment
def generate_engine(chart_parent_path, chart_config, litmus_env):
    engine_filename = chart_parent_path + '/' + 'engine.yaml'

    # Load Jinja2 template
    template = litmus_env.get_template('./templates/experiment_engine.tmpl')
    output_from_parsed_template = template.render(chart_config)
    with open(engine_filename, "w+") as f:
        f.write(output_from_parsed_template)

# generate_job creates the experiment job manifest
def generate_job(job_parent_path, job_name, job_config, litmus_env):
    job_filename = job_parent_path + '/' + job_name + '-' + 'k8s-job.yml'

    # Load Jinja2 template
    template = litmus_env.get_template('./templates/experiment_k8s_job.tmpl')
    output_from_parsed_template = template.render(job_config)
    with open(job_filename, "w+") as f:
        f.write(output_from_parsed_template)

# generate_chaoslib creates the chaoslib for the experiment
def generate_chaoslib(chaoslib_parent_path, chaoslib_name, chaoslib_config, litmus_env):
    chaoslib_filename = chaoslib_parent_path + '/' + chaoslib_name + '.go'

    if os.path.isdir(chaoslib_parent_path) != True:
        os.makedirs(chaoslib_parent_path)
            
    # Load Jinja2 template
    template = litmus_env.get_template('./templates/chaoslib_tmpl')
    output_from_parsed_template = template.render(chaoslib_config)
    with open(chaoslib_filename, "w+") as f:
        f.write(output_from_parsed_template)

# generate_environment creates the environment for the experiment
def generate_environment(environment_parent_path, environment_config, litmus_env):
    environment_filename = environment_parent_path + '/environment.go'

    if os.path.isdir(environment_parent_path) != True:
        os.makedirs(environment_parent_path)
            
    # Load Jinja2 template
    template = litmus_env.get_template('./templates/environment.tmpl')
    output_from_parsed_template = template.render(environment_config)
    with open(environment_filename, "w+") as f:
        f.write(output_from_parsed_template)

# generate_types creates the types.go for the experiment
def generate_types(types_parent_path, types_config, litmus_env):
    types_filename = types_parent_path + '/types.go'

    if os.path.isdir(types_parent_path) != True:
        os.makedirs(types_parent_path)
            
    # Load Jinja2 template
    template = litmus_env.get_template('./templates/types.tmpl')
    output_from_parsed_template = template.render(types_config)
    with open(types_filename, "w+") as f:
        f.write(output_from_parsed_template)

# generate_experiment creates the expriment.go file
def generate_experiment(experiment_parent_path, experiment_name, experiment_config, litmus_env):
    ansible_logic_filename = experiment_parent_path + '/' + experiment_name + '.go'

    # Load Jinja2 template
    template = litmus_env.get_template('./templates/experiment.tmpl')
    output_from_parsed_template = template.render(experiment_config)
    with open(ansible_logic_filename, "w+") as f:
        f.write(output_from_parsed_template)

# generate_package creates the package manifest
def generate_package(package_parent_path, package_name):
    package_filename = package_parent_path + '/' + package_name + '.' + 'package.yaml'
    print(package_filename)
    with open(package_filename, "w+") as f:
        f.write('packageName: ' + package_name + '\n' + 'experiments:')

def main():
    # Required Arguments 
    parser = argparse.ArgumentParser()
    parser.add_argument("-a", "--attributes_file", required=True, 
                        help="metadata to generate chartserviceversion yaml")
    parser.add_argument("-t", "--generate_type", required=True, 
                        help="scaffold a new chart or experiment into existing chart")
    # Optional Arguments
    parser.add_argument("-c", "--chart_name", required=False,
                        help="existing chart name to which experiment belongs, defaults to 'category' in attributes file")

    args = parser.parse_args()

    entity_metadata_source = args.attributes_file
    entity_type = args.generate_type
    entity_parent = args.chart_name

    # Load data from YAML file into a dictionary
    config = yaml.load(open(entity_metadata_source))
    entity_name = config['name']

    # Store the litmus root from bootstrap folder
    litmus_root = os.path.abspath(os.path.join("..", os.pardir))
    #env = Environment(loader = FileSystemLoader('./'), trim_blocks=True, lstrip_blocks=True, autoescape=True)
    env = Environment(loader = FileSystemLoader('./'), trim_blocks=True, lstrip_blocks=True, autoescape=select_autoescape(['yaml']))

    # if generate_type is chart, only create the chart(top)-level CSV & package manifests 
    if entity_type == 'chart':
        chart_dir = litmus_root + '/experiments/' + entity_name
        if os.path.isdir(chart_dir) != True:
            os.makedirs(chart_dir)
        generate_csv(chart_dir, entity_name, config, env) 
        generate_package(chart_dir, entity_name)

    # if generate_type is experiment, create the litmusbook arefacts (job, experiment.go, cr)
    elif entity_type == 'experiment':
        # if chart_name is not explicitly provided, use "category" from attributes.yaml as chart
        if entity_parent is None:
            experiment_category = config['category']
            chart_dir = litmus_root + '/experiments/' + experiment_category
        else:
            chart_dir = litmus_root + '/experiments/' + entity_parent

        chaoslib_dir = litmus_root + '/chaoslib/litmus/' + config['name']
        environment_dir = litmus_root + '/pkg/' + config['category'] + '/' + config['name'] + '/environment'
        types_dir = litmus_root + '/pkg/' + config['category'] + '/' + config['name'] + '/types'
        # if a folder with specified/derived chart name is not present, create it
        if os.path.isdir(chart_dir) != True:
            os.makedirs(chart_dir)
            # generate csv for the freshly created chart folder
            generate_csv(chart_dir, experiment_category, config, env)

            # generate package for the freshly created chart folder
            generate_package(chart_dir, experiment_category)

        # create experiment folder inside the chart folder
        experiment_dir = chart_dir + '/' + entity_name
        if os.path.isdir(experiment_dir) != True:
            os.makedirs(experiment_dir)

        # generate experiment csv
        generate_csv(experiment_dir, entity_name, config, env)

        # generate experiment-custom-resource
        generate_chart(experiment_dir, config, env)

        # generate experiment specific rbac
        generate_rbac(experiment_dir, config, env)

        # generate experiment specific chaos engine
        generate_engine(experiment_dir, config, env)

        # generate experiment job
        generate_job(experiment_dir, entity_name, config, env)

        # generate experiment.go
        generate_experiment(experiment_dir, entity_name, config, env)

        # generate chaoslib
        generate_chaoslib(chaoslib_dir, entity_name, config, env)

        # generate environment.go 
        generate_environment(environment_dir, config, env)

        # generate environment.go 
        generate_types(types_dir, config, env)


if __name__=="__main__":
    main()